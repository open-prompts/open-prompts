package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"
	"open-prompts/backend/internal/repository"

	"github.com/lib/pq"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type PromptService struct {
	pb.UnimplementedPromptServiceServer
	PromptRepo            repository.PromptRepository
	TemplateRepo          repository.TemplateRepository
	TemplateVersionRepo   repository.TemplateVersionRepository
	TemplateAliasRepo     repository.TemplateAliasRepository
	NotificationRepo      repository.NotificationRepository
	NotificationPrefsRepo repository.NotificationPreferencesRepository
	UserRepo              repository.UserRepository
	CommentRepo           repository.CommentRepository
	ReviewRepo            repository.ReviewRepository
	FollowRepo            repository.FollowRepository
	AnalyticsRepo         AnalyticsRepo
	AppealRepo            repository.AppealRepository
	ReportRepo            repository.ReportRepository
	AdminRepo             repository.AdminRepository
	Redis                 RedisStore

	// settingsCache caches platform settings (review thresholds) for
	// settingsCacheTTL to keep the publish hot path free of per-request
	// settings queries. Writes through AdminService invalidate it.
	settingsMu     sync.Mutex
	settingsCache  map[string]string
	settingsExpiry time.Time
}

// settingsCacheTTL bounds platform-settings staleness (architecture §12).
const settingsCacheTTL = 60 * time.Second

// Platform setting keys used by the gating logic.
const (
	settingApprovalThreshold = "review_approval_threshold"
	settingRegateCount       = "review_regate_count"
	settingAgingHours        = "review_aging_hours"
	settingWarningTaxonomy   = "content_warning_taxonomy"
)

// defaultWarningTaxonomy is the built-in fallback warning taxonomy used when
// the platform_settings value is unavailable (fresh installs always seed the
// row, so this is a resilience fallback).
var defaultWarningTaxonomy = []string{"mature", "violence", "profanity", "sensitive"}

// defaultReviewSettings are used when the settings store is unavailable
// (fresh installs always seed the rows, so this is a resilience fallback).
var defaultReviewSettings = map[string]int{
	settingApprovalThreshold: 3,
	settingRegateCount:       3,
	settingAgingHours:        48,
}

func NewPromptService(
	promptRepo repository.PromptRepository,
	templateRepo repository.TemplateRepository,
	templateVersionRepo repository.TemplateVersionRepository,
	templateAliasRepo repository.TemplateAliasRepository,
	notificationRepo repository.NotificationRepository,
	userRepo repository.UserRepository,
	commentRepo repository.CommentRepository,
	redis RedisStore,
) *PromptService {
	return &PromptService{
		PromptRepo:          promptRepo,
		TemplateRepo:        templateRepo,
		TemplateVersionRepo: templateVersionRepo,
		TemplateAliasRepo:   templateAliasRepo,
		NotificationRepo:    notificationRepo,
		UserRepo:            userRepo,
		CommentRepo:         commentRepo,
		Redis:               redis,
		settingsCache:       make(map[string]string),
	}
}

// SetReviewRepo wires the review repository after construction (kept
// separate from the constructor so existing call sites stay unchanged).
func (s *PromptService) SetReviewRepo(repo repository.ReviewRepository) {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	s.ReviewRepo = repo
}

// SetFollowRepo wires the follow repository after construction (nil-safe;
// owner enrichment is skipped when absent).
func (s *PromptService) SetFollowRepo(repo repository.FollowRepository) {
	s.FollowRepo = repo
}

// SetNotificationPrefsRepo wires the notification-preferences repository
// after construction (nil-safe; the preference check fails open when nil).
func (s *PromptService) SetNotificationPrefsRepo(repo repository.NotificationPreferencesRepository) {
	s.NotificationPrefsRepo = repo
}

// SetAnalyticsRepo wires the analytics repository after construction
// (nil-safe; analytics methods return errors when absent).
func (s *PromptService) SetAnalyticsRepo(repo AnalyticsRepo) {
	s.AnalyticsRepo = repo
}

// SetAppealRepo wires the appeal repository after construction (nil-safe;
// appeal methods return errors when absent).
func (s *PromptService) SetAppealRepo(repo repository.AppealRepository) {
	s.AppealRepo = repo
}

// SetReportRepo wires the report repository after construction (nil-safe;
// report methods return errors when absent).
func (s *PromptService) SetReportRepo(repo repository.ReportRepository) {
	s.ReportRepo = repo
}

// SetAdminRepo wires the admin repository after construction (nil-safe;
// appeal-create audit is skipped when absent).
func (s *PromptService) SetAdminRepo(repo repository.AdminRepository) {
	s.AdminRepo = repo
}

// writeAppealAudit persists an appeal audit entry (best-effort; failures are
// logged, never fatal). The actor is the appellant for appeal.create.
func (s *PromptService) writeAppealAudit(ctx context.Context, actorID string, action *models.ModerationAction, appealID string) {
	if s.AdminRepo == nil {
		return
	}
	entry := &models.AdminAuditLog{
		ActorID:    actorID,
		ActorEmail: actorID,
		Action:     "appeal.create",
		TargetType: "template",
		TargetID:   action.TemplateID,
		Reason:     "appeal=" + appealID,
	}
	if err := s.AdminRepo.InsertAuditLog(ctx, entry); err != nil {
		zap.S().Errorf("PromptService: failed to write appeal.create audit (appeal=%s): %v", appealID, err)
	}
}

// writeRepublishAudit records the owner re-publishing an admin-unpublished
// template (OQ-5). Best-effort; failures are logged, never fatal.
func (s *PromptService) writeRepublishAudit(ctx context.Context, actorID, templateID string) {
	if s.AdminRepo == nil {
		return
	}
	entry := &models.AdminAuditLog{
		ActorID:    actorID,
		ActorEmail: actorID,
		Action:     "template.republished_after_moderation",
		TargetType: "template",
		TargetID:   templateID,
		Reason:     "owner re-published after admin unpublish",
	}
	if err := s.AdminRepo.InsertAuditLog(ctx, entry); err != nil {
		zap.S().Errorf("PromptService: failed to write republished_after_moderation audit (template=%s): %v", templateID, err)
	}
}

// isLocked reports whether a template is locked by moderators (FR-3). Locked
// templates block all mutations (edit, comment, fork, like/favorite/rating,
// watch, preset save) while staying publicly visible.
func isLocked(t *models.Template) bool {
	return t != nil && t.ModerationStatus == "locked"
}

// getSettingInt returns a platform setting as an int, through the 60s
// cache. Falls back to the built-in default when the store is unavailable.
func (s *PromptService) getSettingInt(ctx context.Context, key string) int {
	s.settingsMu.Lock()
	if s.settingsCache != nil && time.Now().Before(s.settingsExpiry) {
		if v, ok := s.settingsCache[key]; ok {
			s.settingsMu.Unlock()
			if n, err := strconv.Atoi(v); err == nil {
				return n
			}
			return defaultReviewSettings[key]
		}
	}
	s.settingsMu.Unlock()

	if s.ReviewRepo == nil {
		return defaultReviewSettings[key]
	}
	value, err := s.ReviewRepo.GetSetting(ctx, key)
	if err != nil || value == "" {
		return defaultReviewSettings[key]
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return defaultReviewSettings[key]
	}

	s.settingsMu.Lock()
	if s.settingsCache == nil {
		s.settingsCache = make(map[string]string)
	}
	s.settingsCache[key] = value
	s.settingsExpiry = time.Now().Add(settingsCacheTTL)
	s.settingsMu.Unlock()
	return n
}

// InvalidateSettingsCache clears the platform-settings cache. Called by
// AdminService.UpdatePlatformSetting so new values apply immediately.
func (s *PromptService) InvalidateSettingsCache() {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	s.settingsCache = make(map[string]string)
	s.settingsExpiry = time.Time{}
}

// taxonomy returns the config-driven warning keys (from platform_settings,
// cached 60s). Falls back to the built-in default on cache miss or when the
// stored value is not a valid JSON array of strings.
func (s *PromptService) taxonomy(ctx context.Context) []string {
	if s.ReviewRepo == nil {
		return defaultWarningTaxonomy
	}
	value, err := s.ReviewRepo.GetSetting(ctx, settingWarningTaxonomy)
	if err != nil || value == "" {
		return defaultWarningTaxonomy
	}
	var keys []string
	if err := json.Unmarshal([]byte(value), &keys); err != nil {
		return defaultWarningTaxonomy
	}
	if len(keys) == 0 {
		return defaultWarningTaxonomy
	}
	return keys
}

// isGated returns true when the creator's next publish must enter review.
// Gated iff approved_template_count < N OR review_gated_remaining > 0.
// Creators with no review store wired (e.g. unit tests) are never gated.
func (s *PromptService) isGated(ctx context.Context, userID string) bool {
	if s.ReviewRepo == nil {
		return false
	}
	approved, gatedRemaining, err := s.ReviewRepo.GetUserGatingState(ctx, userID)
	if err != nil {
		// Fail closed so an infra error cannot silently publish unreviewed
		// content (the publish then surfaces the error to the caller).
		zap.S().Warnf("isGated: failed to load gating state for %s: %v", userID, err)
		return true
	}
	threshold := s.getSettingInt(ctx, settingApprovalThreshold)
	// Threshold 0 disables gating entirely (every publish goes live).
	if threshold <= 0 {
		return false
	}
	return approved < int32(threshold) || gatedRemaining > 0
}

// createReviewSubmission inserts the queue row for a gated publish and
// decrements review_gated_remaining when > 0. A failure here fails the
// whole publish: a gated template must never exist without its queue row
// (detailed design §10 constraint 7).
func (s *PromptService) createReviewSubmission(ctx context.Context, templateID, userID string) error {
	if s.ReviewRepo == nil {
		return nil
	}
	attempt := int32(1)
	if latest, err := s.ReviewRepo.GetLatestSubmission(ctx, templateID); err == nil && latest != nil {
		attempt = latest.AttemptNumber + 1
	} else if err != nil && err != repository.ErrSubmissionNotFound {
		return status.Errorf(codes.Internal, "failed to load review history: %v", err)
	}
	if err := s.ReviewRepo.CreateSubmission(ctx, &models.ReviewSubmission{
		TemplateID:    templateID,
		CreatorID:     userID,
		AttemptNumber: attempt,
		Status:        "pending",
		SubmittedAt:   time.Now(),
	}); err != nil {
		return status.Errorf(codes.Internal, "failed to create review submission: %v", err)
	}
	if err := s.ReviewRepo.DecrementGatedRemaining(ctx, userID); err != nil {
		zap.S().Warnf("gated publish: failed to decrement review_gated_remaining for %s: %v", userID, err)
	}
	return nil
}

// createEngagementNotification emits a like/favorite/fork notification to the
// template owner. It is strictly best-effort (FR-7): self-actions are
// suppressed (FR-2) and any failure is logged and swallowed so the triggering
// engagement is never affected.
func (s *PromptService) createEngagementNotification(ctx context.Context, notificationType, templateID, ownerID, actorID string) {
	if s.NotificationRepo == nil || s.UserRepo == nil {
		return
	}
	if ownerID == actorID {
		return
	}
	if !prefsAllow(ctx, s.NotificationPrefsRepo, ownerID, notificationType) {
		return // suppressed by preference
	}
	actor, err := s.UserRepo.GetByID(ctx, actorID)
	if err != nil || actor == nil {
		zap.S().Warnf("notification hook: failed to load actor %s: %v", actorID, err)
		return
	}
	displayName := actor.DisplayName
	if displayName == "" {
		displayName = actor.ID
	}
	if err := s.NotificationRepo.Create(ctx, &models.Notification{
		RecipientID:      ownerID,
		ActorID:          actorID,
		TemplateID:       templateID,
		Type:             notificationType,
		ActorDisplayName: displayName,
	}); err != nil {
		zap.S().Errorf("notification hook: failed to create %s notification (template=%s actor=%s): %v",
			notificationType, templateID, actorID, err)
	}
}

// --- Template RPCs ---

func (s *PromptService) CreateTemplate(ctx context.Context, req *pb.CreateTemplateRequest) (*pb.CreateTemplateResponse, error) {
	// Retrieve User ID from context (injected by AuthInterceptor)
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	zap.S().Infof("PromptService.CreateTemplate: user_id=%s title=%s", userID, req.Title)

	// Map Visibility
	visibility := "private"
	if req.Visibility == pb.Visibility_VISIBILITY_PUBLIC {
		visibility = "public"
	}

	// Map Type
	typeStr := "user"
	if req.Type == pb.TemplateType_TEMPLATE_TYPE_SYSTEM {
		typeStr = "system"
	}

	language := req.Language
	if language == "" {
		language = "en"
	}

	// Pre-publication review gating: a public publish by a gated creator
	// enters the review queue instead of going live. Private creates are
	// never gated.
	moderationStatus := "normal"
	var publishedAt sql.NullTime
	if visibility == "public" {
		if s.isGated(ctx, userID) {
			moderationStatus = "in_review"
		} else {
			publishedAt = sql.NullTime{Time: time.Now(), Valid: true}
		}
	}

	// Validate each requested content warning against the config-driven
	// taxonomy (constraint 8: never trust the client's strings).
	taxonomy := s.taxonomy(ctx)
	for _, w := range req.ContentWarnings {
		if !slices.Contains(taxonomy, w) {
			return nil, status.Errorf(codes.InvalidArgument, "unknown content warning %q", w)
		}
	}

	template := &models.Template{
		OwnerID:          userID, // Use the authenticated user ID
		Title:            req.Title,
		Description:      sql.NullString{String: req.Description, Valid: req.Description != ""},
		Visibility:       visibility,
		Type:             typeStr,
		Tags:             pq.StringArray(req.Tags),
		Category:         sql.NullString{String: req.Category, Valid: req.Category != ""},
		Language:         language,
		ModerationStatus: moderationStatus,
		PublishedAt:      publishedAt,
		ContentWarnings:  pq.StringArray(req.ContentWarnings),
		LockedWarnings:   nil, // a fresh template has no moderator-applied warnings
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if err := s.TemplateRepo.Create(ctx, template); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create template: %v", err)
	}

	// Create Version 1
	version := &models.TemplateVersion{
		TemplateID: template.ID,
		Version:    1,
		Content:    req.Content,
		CreatedAt:  time.Now(),
	}

	if err := s.TemplateVersionRepo.Create(ctx, version); err != nil {
		// Cleanup template? For now, just fail.
		return nil, status.Errorf(codes.Internal, "failed to create template version: %v", err)
	}

	// A gated publish must be atomic with its submission row: if the
	// submission insert fails, the publish fails (constraint 7).
	if moderationStatus == "in_review" {
		if err := s.createReviewSubmission(ctx, template.ID, userID); err != nil {
			return nil, err
		}
	}

	// Auto-create 'latest' alias
	_ = s.TemplateAliasRepo.Create(ctx, &models.TemplateAlias{
		TemplateID: template.ID,
		AliasName:  "latest",
		VersionID:  version.ID,
	})

	return &pb.CreateTemplateResponse{
		Template: s.templateModelToProto(template),
		Version:  s.versionModelToProto(version),
	}, nil
}

// ForkTemplate creates a copy of a template for the current user.
func (s *PromptService) ForkTemplate(ctx context.Context, templateID string) (*pb.CreateTemplateResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// 1. Get Source Template
	sourceTpl, err := s.TemplateRepo.Get(ctx, templateID, "")
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "source template not found")
	}
	// Locked templates block forks (FR-3).
	if isLocked(sourceTpl) {
		return nil, status.Error(codes.PermissionDenied, "this template is locked by moderators")
	}

	// 2. Get Source Latest Version
	sourceVer, err := s.TemplateVersionRepo.GetLatest(ctx, templateID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "source template has no versions")
	}

	zap.S().Infof("Forking template %s version %d to user %s", templateID, sourceVer.Version, userID)

	// 3. Create New Template
	newTpl := &models.Template{
		OwnerID:     userID,
		Title:       sourceTpl.Title,
		Description: sourceTpl.Description,
		Visibility:  "private", // Always private on fork
		Type:        "user",
		Tags:        sourceTpl.Tags,
		Category:    sourceTpl.Category,
		// Forks start private and are never gated; moderation_status must
		// satisfy the CHECK constraint.
		ModerationStatus: "normal",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// Preserve language from source template; default to 'en' if missing
	if sourceTpl.Language != "" {
		newTpl.Language = sourceTpl.Language
	} else {
		newTpl.Language = "en"
	}

	if err := s.TemplateRepo.Create(ctx, newTpl); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create forked template: %v", err)
	}

	// 4. Create New Version (copy content)
	newVer := &models.TemplateVersion{
		TemplateID: newTpl.ID,
		Version:    1,
		Content:    sourceVer.Content,
		CreatedAt:  time.Now(),
	}

	if err := s.TemplateVersionRepo.Create(ctx, newVer); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create forked version: %v", err)
	}

	// Notify the source template owner (best-effort, self-suppressed).
	s.createEngagementNotification(ctx, "fork", templateID, sourceTpl.OwnerID, userID)

	return &pb.CreateTemplateResponse{
		Template: s.templateModelToProto(newTpl),
		Version:  s.versionModelToProto(newVer),
	}, nil
}

// UpdateTemplate updates an existing template.
func (s *PromptService) UpdateTemplate(ctx context.Context, req *pb.UpdateTemplateRequest) (*pb.UpdateTemplateResponse, error) {
	zap.S().Infof("PromptService.UpdateTemplate: template_id=%s", req.TemplateId)
	if len(req.ChangeNote) > 200 {
		return nil, status.Errorf(codes.InvalidArgument, "change_note must be at most 200 characters")
	}
	// AuthZ: derive the caller from context; never trust client-supplied owner_id.
	// Fetch with the caller's id so in_review/rejected templates stay visible
	// (and editable) to their owner (FR-3/FR-6); non-owners still get 404.
	userID, _ := GetUserIDFromContext(ctx)
	template, err := s.TemplateRepo.Get(ctx, req.TemplateId, userID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "template not found")
	}

	if err := authorizeOwnerOrAdmin(ctx, template.OwnerID); err != nil {
		return nil, err
	}

	// Locked templates block edits (FR-3): the owner may view but not modify
	// while under review.
	if isLocked(template) {
		return nil, status.Error(codes.PermissionDenied, "this template is locked by moderators")
	}

	// Capture the pre-update visibility BEFORE the field updates below
	// overwrite it, so a private→public transition is detectable.
	wasPublic := template.Visibility == "public"
	wasRejected := template.ModerationStatus == "rejected"
	wasUnpublishedByAdmin := template.ModerationStatus == "unpublished_by_admin"

	// Update fields
	if req.Title != "" {
		template.Title = req.Title
	}
	// Allow clearing Description by sending empty string
	template.Description = sql.NullString{String: req.Description, Valid: true}

	// Missing field updates added:
	if req.Visibility != pb.Visibility_VISIBILITY_UNSPECIFIED {
		template.Visibility = req.Visibility.String()
		// Also handle short strings "public"/"private" if pb enum strings differ?
		// The PB enum String() returns "VISIBILITY_PUBLIC".
		// The DB likely expects "public" or "private".
		// Let's check model or DB schema.
		// Assuming DB expects lowercase based on other code.
		switch req.Visibility {
		case pb.Visibility_VISIBILITY_PUBLIC:
			template.Visibility = "public"
		case pb.Visibility_VISIBILITY_PRIVATE:
			template.Visibility = "private"
		}
	}

	// Allow clearing Category by sending empty string
	template.Category = sql.NullString{String: req.Category, Valid: true}

	if req.Language != "" {
		template.Language = req.Language
	}

	// Allow clearing Tags by sending empty list
	// Note: this assumes UpdateTemplate is always called with full tag list
	template.Tags = req.Tags

	// Content warnings: enforce the creator-immutable lock (FR-8) and
	// validate the incoming set against the taxonomy (constraint 8).
	// A locked warning must remain present in the incoming set; the creator
	// may freely add/remove their own self-declared warnings.
	locked := template.LockedWarnings
	incoming := req.ContentWarnings
	for _, lw := range locked {
		if !slices.Contains(incoming, lw) {
			return nil, status.Errorf(codes.PermissionDenied,
				"warning %q was applied by moderation and cannot be removed", lw)
		}
	}
	taxonomy := s.taxonomy(ctx)
	for _, w := range incoming {
		if !slices.Contains(taxonomy, w) {
			return nil, status.Errorf(codes.InvalidArgument, "unknown content warning %q", w)
		}
	}
	template.ContentWarnings = pq.StringArray(incoming)
	template.LockedWarnings = locked // preserved; creator cannot change it

	// Pre-publication review gating on the publish transition: a private→public
	// transition — or a rejected template being re-published (FR-6 resubmission;
	// a rejected template keeps visibility=public, so the transition is detected
	// via moderation_status) — by a gated creator enters review. Edits that do
	// not cross the publish boundary leave moderation_status untouched
	// (FR-3/FR-6: in_review/rejected stay editable; FR-14: exempt creators'
	// published edits stay live).
	template.UpdatedAt = time.Now()
	transitioningToPublic := !wasPublic && template.Visibility == "public"
	resubmittingRejected := wasPublic && wasRejected && template.Visibility == "public"
	// Re-publish after admin unpublish (OQ-5): the owner re-shares an
	// admin-unpublished template via the normal share flow. The template's
	// visibility is already public (unpublish only flips moderation_status),
	// so the re-share is detected by the moderation_status + public
	// visibility combination. Moderation is corrective — the owner can
	// re-publish immediately.
	republishingAfterUnpublish := wasUnpublishedByAdmin && template.Visibility == "public"
	if transitioningToPublic || resubmittingRejected || republishingAfterUnpublish {
		if s.isGated(ctx, userID) {
			template.ModerationStatus = "in_review"
			template.PublishedAt = sql.NullTime{}
		} else {
			template.ModerationStatus = "normal"
			if !template.PublishedAt.Valid {
				template.PublishedAt = sql.NullTime{Time: time.Now(), Valid: true}
			}
		}
	}

	if err := s.TemplateRepo.Update(ctx, template); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update template: %v", err)
	}

	// Re-publish marker (OQ-5): when the owner re-publishes an
	// admin-unpublished template via the normal share flow, write an audit
	// event so the pattern is visible in the audit trail. The owner can
	// re-publish immediately (moderation is corrective); repeated violations
	// escalate to a ban.
	if wasUnpublishedByAdmin && template.Visibility == "public" && template.ModerationStatus == "normal" {
		if s.AdminRepo != nil {
			s.writeRepublishAudit(ctx, userID, template.ID)
		}
	}

	// A gated publish transition must be atomic with its submission row
	// (constraint 7): attempt = latest + 1 (or 1 when none).
	if (transitioningToPublic || resubmittingRejected) && template.ModerationStatus == "in_review" {
		if err := s.createReviewSubmission(ctx, template.ID, userID); err != nil {
			return nil, err
		}
	}

	// Create new version
	latest, err := s.TemplateVersionRepo.GetLatest(ctx, template.ID)

	// Check if content has changed
	if err == nil && latest != nil && latest.Content == req.Content {
		// Content hasn't changed, so don't create a new version
		return &pb.UpdateTemplateResponse{
			Template:   s.templateModelToProto(template),
			NewVersion: s.versionModelToProto(latest), // Return latest
		}, nil
	}

	newVersionNum := 1
	if err == nil && latest != nil {
		newVersionNum = int(latest.Version) + 1
	}

	newVersion := &models.TemplateVersion{
		TemplateID: template.ID,
		Version:    int32(newVersionNum),
		Content:    req.Content,
		ChangeNote: sql.NullString{String: req.ChangeNote, Valid: req.ChangeNote != ""},
		CreatedAt:  time.Now(),
	}

	if err := s.TemplateVersionRepo.Create(ctx, newVersion); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create new version: %v", err)
	}

	// Upsert 'latest' alias
	_ = s.TemplateAliasRepo.Upsert(ctx, &models.TemplateAlias{
		TemplateID: req.TemplateId,
		AliasName:  "latest",
		VersionID:  newVersion.ID,
	})

	return &pb.UpdateTemplateResponse{
		Template:   s.templateModelToProto(template),
		NewVersion: s.versionModelToProto(newVersion),
	}, nil
}

// GetTemplate retrieves a template by ID.
func (s *PromptService) GetTemplate(ctx context.Context, req *pb.GetTemplateRequest) (*pb.GetTemplateResponse, error) {
	zap.S().Infof("PromptService.GetTemplate: id=%s", req.Id)
	userID, _ := GetUserIDFromContext(ctx)
	template, err := s.TemplateRepo.Get(ctx, req.Id, userID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "template not found")
	}

	latest, _ := s.TemplateVersionRepo.GetLatest(ctx, template.ID)

	resp := &pb.GetTemplateResponse{
		Template:      s.templateModelToProto(template),
		LatestVersion: s.versionModelToProto(latest),
	}

	// moderation_reason is owner-only: blank it for anonymous/non-owner
	// viewers so it never leaks (e.g. a restored template's restore reason).
	clearModerationReasonForNonOwner(resp.Template, userID)

	// Rating distribution (index 0 -> 1★ ... index 4 -> 5★). Best-effort:
	// a distribution failure must not fail the whole detail read.
	if dist, err := s.TemplateRepo.GetRatingDistribution(ctx, template.ID); err == nil {
		resp.RatingDistribution = dist[:]
	}

	// Owner enrichment: public identity + is_following for the viewer.
	if s.UserRepo != nil {
		if owner, err := s.UserRepo.GetByID(ctx, template.OwnerID); err == nil && owner != nil {
			ownerInfo := &pb.OwnerInfo{
				Id:            owner.ID,
				DisplayName:   owner.DisplayName,
				Avatar:        owner.Avatar,
				FollowerCount: owner.FollowerCount,
			}
			if userID != "" && s.FollowRepo != nil {
				if isFollowing, err := s.FollowRepo.IsFollowing(ctx, userID, owner.ID); err == nil {
					ownerInfo.IsFollowing = isFollowing
				}
			}
			resp.Owner = ownerInfo
		}
	}

	return resp, nil
}

// normalizeKeyword trims, collapses internal whitespace, and caps at 100
// runes (not bytes — CJK-safe). Empty result means "no keyword filter".
func normalizeKeyword(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) > 100 {
		s = string([]rune(s)[:100])
	}
	return s
}

// normalizeSort returns a whitelisted sort value; anything else -> "newest".
func normalizeSort(s string) string {
	switch s {
	case "newest", "most_liked", "most_favorited", "trending", "top_rated", "updated":
		return s
	default:
		return "newest"
	}
}

// ListTemplates retrieves a list of templates.
func (s *PromptService) ListTemplates(ctx context.Context, req *pb.ListTemplatesRequest) (*pb.ListTemplatesResponse, error) {
	userID, _ := GetUserIDFromContext(ctx)
	keyword := normalizeKeyword(req.Keyword)
	sort := normalizeSort(req.Sort)
	zap.S().Infof("PromptService.ListTemplates: page_size=%d page_token=%s owner_id=%s visibility=%s user_id=%s keyword_len=%d sort=%s", req.PageSize, req.PageToken, req.OwnerId, req.Visibility, userID, len(keyword), sort)

	limit := int(req.PageSize)
	if limit <= 0 {
		limit = 10
	}

	// Helper to fetch templates and versions
	fetch := func(limit, offset int, filters map[string]interface{}) ([]*pb.Template, string, error) {
		if userID != "" {
			filters["current_user_id"] = userID
		}
		zap.S().Infof("ListTemplates.fetch: limit=%d offset=%d filters=%v", limit, offset, filters)
		templates, err := s.TemplateRepo.List(ctx, limit, offset, filters)
		if err != nil {
			zap.S().Errorf("ListTemplates.fetch: error listing templates: %v", err)
			return nil, "", err
		}
		zap.S().Infof("ListTemplates.fetch: templates returned=%d", len(templates))
		var pbTemplates []*pb.Template
		for _, t := range templates {
			pbT := s.templateModelToProto(t)
			clearModerationReasonForNonOwner(pbT, userID)
			latest, err := s.TemplateVersionRepo.GetLatest(ctx, t.ID)
			if err == nil {
				pbT.LatestVersion = s.versionModelToProto(latest)
			}
			pbTemplates = append(pbTemplates, pbT)
		}
		nextToken := ""
		if len(templates) == limit {
			nextToken = strconv.Itoa(offset + limit)
		}
		return pbTemplates, nextToken, nil
	}

	// count runs a COUNT query with the same filters (minus keys the
	// condition builder ignores). Only called on the first page of a query.
	count := func(filters map[string]interface{}) int32 {
		countFilters := make(map[string]interface{}, len(filters))
		for k, v := range filters {
			if k == "current_user_id" {
				continue
			}
			countFilters[k] = v
		}
		n, err := s.TemplateRepo.CountTemplates(ctx, countFilters)
		if err != nil {
			zap.S().Warnf("ListTemplates.count: error counting templates: %v", err)
			return 0
		}
		return n
	}

	// applySearch adds keyword/sort filters shared by all list paths.
	applySearch := func(filters map[string]interface{}) {
		if keyword != "" {
			filters["keyword"] = keyword
		}
		filters["sort"] = sort
		if sort == "top_rated" {
			filters["top_rated_threshold"] = s.topRatedMinRatings()
		}
	}

	// applyWarnedFilter adds the exclude_warned filter to a public stream.
	// It is deliberately NOT applied to private streams (a private template's
	// warnings are only visible to its owner).
	applyWarnedFilter := func(filters map[string]interface{}) {
		if req.ExcludeWarned {
			filters["exclude_warned"] = true
		}
	}

	// SPECIAL HANDLING: My Likes / My Favorites (Treat as single stream)
	if userID != "" && (req.MyLikes || req.MyFavorites) {
		offset := 0
		if req.PageToken != "" {
			offset, _ = strconv.Atoi(req.PageToken)
		}
		filters := make(map[string]interface{})
		if req.MyLikes {
			filters["my_likes"] = true
		}
		if req.MyFavorites {
			filters["my_favorites"] = true
		}
		if req.Category != "" {
			filters["category"] = req.Category
		}
		if req.Language != "" {
			filters["language"] = req.Language
		}
		if len(req.Tags) > 0 {
			filters["tags"] = req.Tags
		}
		applySearch(filters)
		applyWarnedFilter(filters)

		templates, nextToken, err := fetch(limit, offset, filters)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to list templates: %v", err)
		}
		var totalCount int32
		if req.PageToken == "" {
			totalCount = count(filters)
		}
		return &pb.ListTemplatesResponse{Templates: templates, NextPageToken: nextToken, TotalCount: totalCount}, nil
	}

	// 1. No Token: Return Public Only
	if userID == "" {
		offset := 0
		if req.PageToken != "" {
			offset, _ = strconv.Atoi(req.PageToken)
		}
		filters := make(map[string]interface{})
		filters["visibility"] = "public"
		if req.Category != "" {
			filters["category"] = req.Category
		}
		if req.Language != "" {
			filters["language"] = req.Language
		}
		if len(req.Tags) > 0 {
			filters["tags"] = req.Tags
		}
		if req.OwnerId != "" {
			filters["owner_id"] = req.OwnerId
		}
		applySearch(filters)
		applyWarnedFilter(filters)

		templates, nextToken, err := fetch(limit, offset, filters)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to list templates: %v", err)
		}
		var totalCount int32
		if req.PageToken == "" {
			totalCount = count(filters)
		}
		return &pb.ListTemplatesResponse{Templates: templates, NextPageToken: nextToken, TotalCount: totalCount}, nil
	}

	// 2. Token Present
	// If specific visibility requested, return single list
	if req.Visibility != pb.Visibility_VISIBILITY_UNSPECIFIED {
		offset := 0
		if req.PageToken != "" {
			offset, _ = strconv.Atoi(req.PageToken)
		}
		filters := make(map[string]interface{})
		if req.Visibility == pb.Visibility_VISIBILITY_PUBLIC {
			filters["visibility"] = "public"
		} else {
			filters["visibility"] = "private"
			if req.OwnerId == "" {
				filters["owner_id"] = userID // Implicitly my private
			} else {
				filters["owner_id"] = req.OwnerId // Specific owner private (likely should enforce same user)
				if req.OwnerId != userID {
					// Cannot see others' private
					return &pb.ListTemplatesResponse{}, nil
				}
			}
		}
		if req.Category != "" {
			filters["category"] = req.Category
		}
		if req.Language != "" {
			filters["language"] = req.Language
		}
		if len(req.Tags) > 0 {
			filters["tags"] = req.Tags
		}
		applySearch(filters)
		// exclude_warned applies to the public stream only (FR-6); a private
		// listing is untouched.
		if req.Visibility == pb.Visibility_VISIBILITY_PUBLIC && req.ExcludeWarned {
			filters["exclude_warned"] = true
		}

		templates, nextToken, err := fetch(limit, offset, filters)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to list templates: %v", err)
		}
		var totalCount int32
		if req.PageToken == "" {
			totalCount = count(filters)
		}
		return &pb.ListTemplatesResponse{Templates: templates, NextPageToken: nextToken, TotalCount: totalCount}, nil
	}

	// 3. Token Present AND Visibility Unspecified -> Mixed View
	// When a user is logged in (Token present) and no specific visibility filter is applied,
	// we return a mixed view containing both Public templates and the user's Private templates.
	// Each list is paginated independently, so we return two separate lists and two separate next page tokens (if applicable).
	// To support a single 'next_page_token' in the request, we encode both offsets.
	// Parse tokens. Format: "public_offset:private_offset"
	publicOffset := 0
	privateOffset := 0
	if req.PageToken != "" {
		parts := strings.Split(req.PageToken, ":")
		if len(parts) == 2 {
			// Format is "public:private"
			publicOffset, _ = strconv.Atoi(parts[0])
			privateOffset, _ = strconv.Atoi(parts[1])
		} else {
			// Fallback if parsing fails or old format (single integer),
			// assume 0 for both or try parsing as single offset for backward compatibility.
			if val, err := strconv.Atoi(req.PageToken); err == nil {
				publicOffset = val
				privateOffset = val // Ambiguous, but safe to start both at same offset if previously simple
			}
		}
	}

	// Fetch Public
	publicFilters := map[string]interface{}{"visibility": "public"}
	if req.Category != "" {
		publicFilters["category"] = req.Category
	}
	if req.Language != "" {
		publicFilters["language"] = req.Language
	}
	if len(req.Tags) > 0 {
		publicFilters["tags"] = req.Tags
	}
	// If owner_id is specified in mixed view, we filter public by that owner too
	if req.OwnerId != "" {
		publicFilters["owner_id"] = req.OwnerId
	}
	applySearch(publicFilters)
	applyWarnedFilter(publicFilters)

	publicTemplates, nextPublicToken, err := fetch(limit, publicOffset, publicFilters)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list public templates: %v", err)
	}

	// Fetch Private
	privateFilters := map[string]interface{}{"visibility": "private", "owner_id": userID}
	if req.Category != "" {
		privateFilters["category"] = req.Category
	}
	if req.Language != "" {
		privateFilters["language"] = req.Language
	}
	if len(req.Tags) > 0 {
		privateFilters["tags"] = req.Tags
	}
	applySearch(privateFilters)
	// If request owner_id is specified and is NOT me, I shouldn't see private?
	// But "Mixed" implies "My Private + All Public".
	// If I filter by "Alice", I see Alice's Public. Do I see My Private? No, unless I am Alice.
	// So if OwnerId filter is present and != userID, we skip private fetch.
	var privateTemplates []*pb.Template
	nextPrivateToken := ""

	shouldFetchPrivate := req.OwnerId == "" || req.OwnerId == userID

	if shouldFetchPrivate {
		var err error
		privateTemplates, nextPrivateToken, err = fetch(limit, privateOffset, privateFilters)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to list private templates: %v", err)
		}
	}

	// Construct Response
	// We put Public in "Templates" for backward compatibility/simplicity?
	// Or we use the split fields.
	// Proto: templates (1), private_templates (3).
	// Let's populate: public -> templates, private -> private_templates.

	// Tokens:
	// We need to construct a combined token if both have more pages?
	// Or we return separate tokens?
	// Proto: next_page_token (2), private_next_page_token (4).

	var totalCount, privateTotalCount int32
	if req.PageToken == "" {
		totalCount = count(publicFilters)
		if shouldFetchPrivate {
			privateTotalCount = count(privateFilters)
		}
	}

	return &pb.ListTemplatesResponse{
		Templates:            publicTemplates,
		NextPageToken:        nextPublicToken,
		PrivateTemplates:     privateTemplates,
		PrivateNextPageToken: nextPrivateToken,
		TotalCount:           totalCount,
		PrivateTotalCount:    privateTotalCount,
	}, nil
}

func (s *PromptService) DeleteTemplate(ctx context.Context, req *pb.DeleteTemplateRequest) (*pb.DeleteTemplateResponse, error) {
	zap.S().Infof("PromptService.DeleteTemplate: id=%s owner_id=%s", req.Id, req.OwnerId)
	// Fetch with the caller's id so owners can delete their in_review/
	// rejected templates (FR-3/FR-6); non-owners still get 404.
	userID, _ := GetUserIDFromContext(ctx)
	template, err := s.TemplateRepo.Get(ctx, req.Id, userID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "template not found")
	}

	if err := authorizeOwnerOrAdmin(ctx, template.OwnerID); err != nil {
		return nil, err
	}

	if err := s.TemplateRepo.Delete(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete template: %v", err)
	}

	return &pb.DeleteTemplateResponse{Success: true}, nil
}

// ToggleLikeTemplate toggles the like status of a template.
func (s *PromptService) ToggleLikeTemplate(ctx context.Context, req *pb.ToggleLikeRequest) (*pb.ToggleLikeResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	zap.S().Infof("PromptService.ToggleLikeTemplate: user_id=%s template_id=%s", userID, req.TemplateId)

	// Hidden (in_review/rejected) templates are invisible to non-owners —
	// 404 before touching engagement state (constraint 5).
	tpl, err := s.TemplateRepo.Get(ctx, req.TemplateId, userID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "template not found")
	}
	// Locked templates block new likes (FR-3).
	if isLocked(tpl) {
		return nil, status.Error(codes.PermissionDenied, "this template is locked by moderators")
	}

	isLiked, count, err := s.TemplateRepo.ToggleLike(ctx, userID, req.TemplateId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to toggle like: %v", err)
	}

	// Notify the owner when the like was toggled ON (best-effort).
	if isLiked {
		s.createEngagementNotification(ctx, "like", req.TemplateId, tpl.OwnerID, userID)
	}

	return &pb.ToggleLikeResponse{
		IsLiked:   isLiked,
		LikeCount: count,
	}, nil
}

// ToggleFavoriteTemplate toggles the favorite status of a template.
func (s *PromptService) ToggleFavoriteTemplate(ctx context.Context, req *pb.ToggleFavoriteRequest) (*pb.ToggleFavoriteResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	zap.S().Infof("PromptService.ToggleFavoriteTemplate: user_id=%s template_id=%s", userID, req.TemplateId)

	// Hidden (in_review/rejected) templates are invisible to non-owners —
	// 404 before touching engagement state (constraint 5).
	tpl, err := s.TemplateRepo.Get(ctx, req.TemplateId, userID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "template not found")
	}
	// Locked templates block new favorites (FR-3).
	if isLocked(tpl) {
		return nil, status.Error(codes.PermissionDenied, "this template is locked by moderators")
	}

	isFavorited, count, err := s.TemplateRepo.ToggleFavorite(ctx, userID, req.TemplateId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to toggle favorite: %v", err)
	}

	// Notify the owner when the favorite was toggled ON (best-effort).
	if isFavorited {
		s.createEngagementNotification(ctx, "favorite", req.TemplateId, tpl.OwnerID, userID)
	}

	return &pb.ToggleFavoriteResponse{
		IsFavorited:   isFavorited,
		FavoriteCount: count,
	}, nil
}

// ratingRatePerMinuteDef is the default rating changes per minute per
// account (RATING_RATE_PER_MINUTE).
const ratingRatePerMinuteDef = 20

// topRatedMinRatingsDef is the default minimum rating_count for the
// top_rated sort threshold (TOP_RATED_MIN_RATINGS).
const topRatedMinRatingsDef = 5

// topRatedMinRatings returns the top_rated sort threshold from the
// TOP_RATED_MIN_RATINGS env var, falling back to the default.
func (s *PromptService) topRatedMinRatings() int {
	n, _ := strconv.Atoi(os.Getenv("TOP_RATED_MIN_RATINGS"))
	if n <= 0 {
		return topRatedMinRatingsDef
	}
	return n
}

// checkRatingRate enforces RATING_RATE_PER_MINUTE using a fixed-window
// counter in Redis. Returns (retryAfterSeconds, false) when over limit.
// Fail-open when Redis is unavailable. Only successful changes consume
// quota: the caller invokes this right before the rating mutation.
func (s *PromptService) checkRatingRate(ctx context.Context, userID string) (int, bool) {
	if s.Redis == nil {
		return 0, true
	}
	key := "rating_rate_min:" + userID
	max, _ := strconv.Atoi(os.Getenv("RATING_RATE_PER_MINUTE"))
	if max <= 0 {
		max = ratingRatePerMinuteDef
	}
	window := time.Minute
	v, err := s.Redis.Get(ctx, key)
	if err != nil || v == "" {
		_ = s.Redis.Set(ctx, key, "1:"+strconv.FormatInt(time.Now().Add(window).Unix(), 10), window)
		return 0, true
	}
	var count int
	var expires int64
	if _, err := fmt.Sscanf(v, "%d:%d", &count, &expires); err != nil {
		_ = s.Redis.Set(ctx, key, "1:"+strconv.FormatInt(time.Now().Add(window).Unix(), 10), window)
		return 0, true
	}
	if time.Now().Unix() >= expires { // window elapsed: reset
		_ = s.Redis.Set(ctx, key, "1:"+strconv.FormatInt(time.Now().Add(window).Unix(), 10), window)
		return 0, true
	}
	if count >= max {
		return int(expires - time.Now().Unix()), false
	}
	_ = s.Redis.Set(ctx, key,
		fmt.Sprintf("%d:%d", count+1, expires),
		time.Duration(expires-time.Now().Unix())*time.Second)
	return 0, true
}

// RateTemplate upserts the caller's 1-5 star rating for a template.
func (s *PromptService) RateTemplate(ctx context.Context, req *pb.RateTemplateRequest) (*pb.RateTemplateResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	zap.S().Infof("PromptService.RateTemplate: user_id=%s template_id=%s rating=%d", userID, req.TemplateId, req.Rating)

	// Validate the 1-5 domain.
	if req.Rating < 1 || req.Rating > 5 {
		return nil, status.Errorf(codes.InvalidArgument, "rating must be between 1 and 5")
	}

	// Hidden (in_review/rejected) templates are invisible to non-owners —
	// 404 before touching rating state (constraint 5).
	tpl, err := s.TemplateRepo.Get(ctx, req.TemplateId, userID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "template not found")
	}
	// Locked templates block new ratings (FR-3).
	if isLocked(tpl) {
		return nil, status.Error(codes.PermissionDenied, "this template is locked by moderators")
	}

	// Private templates are invisible to non-owners — 404 (no existence leak).
	if tpl.Visibility == "private" && tpl.OwnerID != userID {
		return nil, status.Errorf(codes.NotFound, "template not found")
	}

	// Owner self-rating is forbidden.
	if tpl.OwnerID == userID {
		return nil, status.Errorf(codes.PermissionDenied, "cannot rate your own template")
	}

	// Banned callers are blocked.
	if s.UserRepo != nil {
		if u, err := s.UserRepo.GetByID(ctx, userID); err == nil && u != nil && u.Status == "banned" {
			return nil, status.Errorf(codes.PermissionDenied, "banned users cannot rate templates")
		}
	}

	// Rate limit — only successful changes consume quota.
	if retryAfter, ok := s.checkRatingRate(ctx, userID); !ok {
		return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded, retry after %d seconds", retryAfter)
	}

	sum, count, err := s.TemplateRepo.RateTemplate(ctx, userID, req.TemplateId, req.Rating)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to rate template: %v", err)
	}

	return ratingResponse(sum, count, req.Rating), nil
}

// DeleteRating removes the caller's rating for a template (idempotent).
func (s *PromptService) DeleteRating(ctx context.Context, req *pb.DeleteRatingRequest) (*pb.RateTemplateResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	zap.S().Infof("PromptService.DeleteRating: user_id=%s template_id=%s", userID, req.TemplateId)

	// Hidden (in_review/rejected) templates are invisible to non-owners —
	// 404 before touching rating state (constraint 5).
	tpl, err := s.TemplateRepo.Get(ctx, req.TemplateId, userID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "template not found")
	}

	// Private templates are invisible to non-owners — 404 (no existence leak).
	if tpl.Visibility == "private" && tpl.OwnerID != userID {
		return nil, status.Errorf(codes.NotFound, "template not found")
	}

	// Rate limit — only successful changes consume quota.
	if retryAfter, ok := s.checkRatingRate(ctx, userID); !ok {
		return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded, retry after %d seconds", retryAfter)
	}

	sum, count, err := s.TemplateRepo.DeleteRating(ctx, userID, req.TemplateId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete rating: %v", err)
	}

	return ratingResponse(sum, count, 0), nil
}

// ratingResponse builds a RateTemplateResponse from fresh aggregates.
func ratingResponse(sum int64, count int32, myRating int32) *pb.RateTemplateResponse {
	avg := 0.0
	if count > 0 {
		avg = float64(sum) / float64(count)
	}
	return &pb.RateTemplateResponse{
		RatingAvg:   avg,
		RatingCount: count,
		MyRating:    myRating,
	}
}

// appealMaxOpen returns the configurable open-appeal limit (APPEAL_MAX_OPEN,
// default 5). Mirrors the RATING_RATE_PER_MINUTE config pattern.
func appealMaxOpen() int {
	max, _ := strconv.Atoi(os.Getenv("APPEAL_MAX_OPEN"))
	if max <= 0 {
		return 5
	}
	return max
}

// checkOpenAppealLimit enforces the simultaneous open-appeal limit using a
// Redis counter (appeal_open:{user_id}). Returns (retryAfterSeconds, false)
// when over limit. Fail-open when Redis is unavailable.
func (s *PromptService) checkOpenAppealLimit(ctx context.Context, userID string) (int, bool) {
	if s.Redis == nil {
		return 0, true
	}
	key := "appeal_open:" + userID
	max := appealMaxOpen()
	window := 24 * time.Hour
	v, err := s.Redis.Get(ctx, key)
	if err != nil || v == "" {
		_ = s.Redis.Set(ctx, key, "1:"+strconv.FormatInt(time.Now().Add(window).Unix(), 10), window)
		return 0, true
	}
	var count int
	var expires int64
	if _, err := fmt.Sscanf(v, "%d:%d", &count, &expires); err != nil {
		_ = s.Redis.Set(ctx, key, "1:"+strconv.FormatInt(time.Now().Add(window).Unix(), 10), window)
		return 0, true
	}
	if time.Now().Unix() >= expires { // window elapsed: reset
		_ = s.Redis.Set(ctx, key, "1:"+strconv.FormatInt(time.Now().Add(window).Unix(), 10), window)
		return 0, true
	}
	if count >= max {
		return int(expires - time.Now().Unix()), false
	}
	_ = s.Redis.Set(ctx, key,
		fmt.Sprintf("%d:%d", count+1, expires),
		time.Duration(expires-time.Now().Unix())*time.Second)
	return 0, true
}

// decrementOpenAppeal decrements the open-appeal counter on resolution.
// Fail-open when Redis is unavailable.
func (s *PromptService) decrementOpenAppeal(ctx context.Context, userID string) {
	if s.Redis == nil {
		return
	}
	key := "appeal_open:" + userID
	v, err := s.Redis.Get(ctx, key)
	if err != nil || v == "" {
		return
	}
	var count int
	var expires int64
	if _, err := fmt.Sscanf(v, "%d:%d", &count, &expires); err != nil {
		return
	}
	if count <= 1 {
		_ = s.Redis.Del(ctx, key)
		return
	}
	_ = s.Redis.Set(ctx, key,
		fmt.Sprintf("%d:%d", count-1, expires),
		time.Duration(expires-time.Now().Unix())*time.Second)
}

// appealToProto maps a models.Appeal (with joined action/template fields) to
// the proto Appeal message.
func appealToProto(a *models.Appeal) *pb.Appeal {
	superseded := a.ActionStatus == "superseded"
	resp := &pb.Appeal{
		Id:                 a.ID,
		ModerationActionId: a.ModerationActionID,
		TemplateId:         a.TemplateID,
		TemplateTitle:      a.TemplateTitle,
		ActionType:         a.ActionType,
		ActionReason:       a.ActionReason,
		ActorDisplayName:   a.ActorEmail,
		Statement:          a.Statement,
		State:              a.State,
		ResolutionNote:     a.ResolutionNote,
		Superseded:         superseded,
	}
	if !a.ActionCreatedAt.IsZero() {
		resp.ActionCreatedAt = timestamppb.New(a.ActionCreatedAt)
	}
	if !a.CreatedAt.IsZero() {
		resp.CreatedAt = timestamppb.New(a.CreatedAt)
	}
	if a.ResolvedAt != nil {
		resp.ResolvedAt = timestamppb.New(*a.ResolvedAt)
	}
	return resp
}

// CreateAppeal opens an appeal against a moderation action on one of the
// caller's templates (FR-1..FR-3, FR-14, FR-16).
func (s *PromptService) CreateAppeal(ctx context.Context, req *pb.CreateAppealRequest) (*pb.Appeal, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if s.AppealRepo == nil {
		return nil, status.Error(codes.Unimplemented, "appeal repository not configured")
	}
	zap.S().Infof("PromptService.CreateAppeal: user_id=%s action=%s", userID, req.ModerationActionId)

	// Validate the statement length (50-2000 chars).
	statement := strings.TrimSpace(req.Statement)
	if len([]rune(statement)) < 50 || len([]rune(statement)) > 2000 {
		return nil, status.Error(codes.InvalidArgument, "statement must be between 50 and 2000 characters")
	}

	// The action must exist and target one of the caller's templates (404 on
	// missing or foreign — no existence leak).
	action, err := s.AppealRepo.GetModerationAction(ctx, req.ModerationActionId)
	if err != nil {
		if err == repository.ErrActionNotFound {
			return nil, status.Error(codes.NotFound, "moderation action not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to load moderation action: %v", err)
	}
	tpl, err := s.TemplateRepo.Get(ctx, action.TemplateID, userID)
	if err != nil || tpl == nil || tpl.OwnerID != userID {
		return nil, status.Error(codes.NotFound, "moderation action not found")
	}

	// A superseded action (undone by a direct restore) can no longer be
	// appealed (FR-14).
	if action.Status == "superseded" {
		return nil, status.Error(codes.AlreadyExists, "this action has been superseded and can no longer be appealed")
	}

	// Reject a duplicate appeal for the same action BEFORE consuming the
	// open-appeal quota (FR-2). Checking here (rather than relying on the
	// unique-violation from CreateAppeal) keeps the Redis counter accurate:
	// a 409 must not increment the open-appeal count.
	if _, err := s.AppealRepo.GetAppealByAction(ctx, action.ID); err == nil {
		return nil, status.Error(codes.AlreadyExists, "this action has already been appealed")
	} else if err != repository.ErrAppealNotFound {
		return nil, status.Errorf(codes.Internal, "failed to check existing appeal: %v", err)
	}

	// Open-appeal rate limit (FR-16).
	if retryAfter, ok := s.checkOpenAppealLimit(ctx, userID); !ok {
		return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded, retry after %d seconds", retryAfter)
	}

	appeal := &models.Appeal{
		ModerationActionID: action.ID,
		AppellantID:        userID,
		Statement:          statement,
		State:              "open",
	}
	if err := s.AppealRepo.CreateAppeal(ctx, appeal); err != nil {
		if err == repository.ErrAppealAlreadyExists {
			return nil, status.Error(codes.AlreadyExists, "this action has already been appealed")
		}
		return nil, status.Errorf(codes.Internal, "failed to create appeal: %v", err)
	}

	// Audit the appeal creation (FR-13). Best-effort.
	s.writeAppealAudit(ctx, userID, action, appeal.ID)

	// Re-read with joined fields for the response.
	full, err := s.AppealRepo.GetAppeal(ctx, appeal.ID)
	if err != nil {
		// Fall back to the minimal appeal.
		return appealToProto(&models.Appeal{
			ID:                 appeal.ID,
			ModerationActionID: appeal.ModerationActionID,
			AppellantID:        appeal.AppellantID,
			Statement:          appeal.Statement,
			State:              appeal.State,
			CreatedAt:          appeal.CreatedAt,
			TemplateID:         action.TemplateID,
			ActionType:         action.ActionType,
			ActionReason:       action.Reason,
			ActorEmail:         action.ActorEmail,
			ActionCreatedAt:    action.CreatedAt,
			ActionStatus:       action.Status,
		}), nil
	}
	return appealToProto(full), nil
}

// ListAppeals returns the caller's own appeals, newest first (FR-5).
func (s *PromptService) ListAppeals(ctx context.Context, req *pb.ListAppealsRequest) (*pb.ListAppealsResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if s.AppealRepo == nil {
		return nil, status.Error(codes.Unimplemented, "appeal repository not configured")
	}
	page := int(req.Page)
	if page < 1 {
		page = 1
	}
	perPage := int(req.PerPage)
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	items, total, err := s.AppealRepo.ListAppeals(ctx, userID, page, perPage)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list appeals: %v", err)
	}
	appeals := make([]*pb.Appeal, 0, len(items))
	for i := range items {
		appeals = append(appeals, appealToProto(&items[i]))
	}
	return &pb.ListAppealsResponse{
		Appeals: appeals,
		Total:   total,
		Page:    int32(page),
		PerPage: int32(perPage),
	}, nil
}

// GetAppeal returns a single appeal (owner-only, FR-5).
func (s *PromptService) GetAppeal(ctx context.Context, req *pb.GetAppealRequest) (*pb.Appeal, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if s.AppealRepo == nil {
		return nil, status.Error(codes.Unimplemented, "appeal repository not configured")
	}
	appeal, err := s.AppealRepo.GetAppeal(ctx, req.AppealId)
	if err != nil {
		if err == repository.ErrAppealNotFound {
			return nil, status.Error(codes.NotFound, "appeal not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get appeal: %v", err)
	}
	if appeal.AppellantID != userID {
		return nil, status.Error(codes.NotFound, "appeal not found")
	}
	// Enrich with action/template context.
	action, err := s.AppealRepo.GetModerationAction(ctx, appeal.ModerationActionID)
	if err == nil {
		appeal.TemplateID = action.TemplateID
		appeal.ActionType = action.ActionType
		appeal.ActionReason = action.Reason
		appeal.ActorEmail = action.ActorEmail
		appeal.ActionCreatedAt = action.CreatedAt
		appeal.ActionStatus = action.Status
		if tpl, terr := s.TemplateRepo.Get(ctx, action.TemplateID, userID); terr == nil && tpl != nil {
			appeal.TemplateTitle = tpl.Title
		}
	}
	return appealToProto(appeal), nil
}

// reportReasonTaxonomy is the fixed set of report reason codes (FR-5).
var reportReasonTaxonomy = map[string]bool{
	"spam": true, "offensive": true, "misleading": true, "privacy": true, "other": true,
}

// reportRatePerHourDef is the default report submissions per user per hour
// (REPORT_RATE_PER_HOUR).
const reportRatePerHourDef = 20

// reportRatePerHour returns the configurable report rate limit
// (REPORT_RATE_PER_HOUR, default 20). Mirrors the RATING_RATE_PER_MINUTE
// config pattern.
func reportRatePerHour() int {
	n, _ := strconv.Atoi(os.Getenv("REPORT_RATE_PER_HOUR"))
	if n <= 0 {
		return reportRatePerHourDef
	}
	return n
}

// checkReportRate enforces REPORT_RATE_PER_HOUR using a sliding-window
// counter in Redis (report_rate:{user_id}). Returns (retryAfterSeconds,
// false) when over limit. Fail-open when Redis is unavailable.
func (s *PromptService) checkReportRate(ctx context.Context, userID string) (int, bool) {
	if s.Redis == nil {
		return 0, true
	}
	key := "report_rate:" + userID
	max := reportRatePerHour()
	window := time.Hour
	v, err := s.Redis.Get(ctx, key)
	if err != nil || v == "" {
		_ = s.Redis.Set(ctx, key, "1:"+strconv.FormatInt(time.Now().Add(window).Unix(), 10), window)
		return 0, true
	}
	var count int
	var expires int64
	if _, err := fmt.Sscanf(v, "%d:%d", &count, &expires); err != nil {
		_ = s.Redis.Set(ctx, key, "1:"+strconv.FormatInt(time.Now().Add(window).Unix(), 10), window)
		return 0, true
	}
	if time.Now().Unix() >= expires { // window elapsed: reset
		_ = s.Redis.Set(ctx, key, "1:"+strconv.FormatInt(time.Now().Add(window).Unix(), 10), window)
		return 0, true
	}
	if count >= max {
		return int(expires - time.Now().Unix()), false
	}
	_ = s.Redis.Set(ctx, key,
		fmt.Sprintf("%d:%d", count+1, expires),
		time.Duration(expires-time.Now().Unix())*time.Second)
	return 0, true
}

// reportToProto maps a models.ContentReport to the proto Report message.
func reportToProto(r *models.ContentReport) *pb.Report {
	resp := &pb.Report{
		Id:       r.ID,
		ItemType: r.ItemType,
		ItemId:   r.ItemID,
		Reason:   r.Reason,
		Note:     r.Note,
		Status:   r.Status,
	}
	if !r.CreatedAt.IsZero() {
		resp.CreatedAt = timestamppb.New(r.CreatedAt)
	}
	return resp
}

// CreateReport creates or updates the caller's report on a public template or
// comment (FR-1..FR-6, FR-14, FR-16).
func (s *PromptService) CreateReport(ctx context.Context, req *pb.CreateReportRequest) (*pb.Report, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if s.ReportRepo == nil {
		return nil, status.Error(codes.Unimplemented, "report repository not configured")
	}
	zap.S().Infof("PromptService.CreateReport: user_id=%s item_type=%s item_id=%s", userID, req.ItemType, req.ItemId)

	// Validate item_type and reason (FR-5) before any DB round-trip.
	if req.ItemType != "template" && req.ItemType != "comment" {
		return nil, status.Error(codes.InvalidArgument, "item_type must be one of: template, comment")
	}
	if !reportReasonTaxonomy[req.Reason] {
		return nil, status.Error(codes.InvalidArgument, "reason must be one of: spam, offensive, misleading, privacy, other")
	}
	note := strings.TrimSpace(req.Note)
	if len([]rune(note)) > 500 {
		return nil, status.Error(codes.InvalidArgument, "note must be at most 500 characters")
	}

	// Banned users cannot report (FR-3).
	if s.UserRepo != nil {
		if u, uerr := s.UserRepo.GetByID(ctx, userID); uerr == nil && u != nil && u.Status == "banned" {
			return nil, status.Error(codes.PermissionDenied, "banned users cannot report content")
		}
	}

	// Load the item; not found or not public -> 404 (indistinguishable).
	// Reporting own content -> 400 (FR-4 backstop).
	switch req.ItemType {
	case "template":
		tpl, terr := s.TemplateRepo.Get(ctx, req.ItemId, userID)
		if terr != nil || tpl == nil {
			return nil, status.Error(codes.NotFound, "item not found")
		}
		if tpl.Visibility != "public" {
			return nil, status.Error(codes.NotFound, "item not found")
		}
		if tpl.OwnerID == userID {
			return nil, status.Error(codes.InvalidArgument, "you cannot report your own content")
		}
	case "comment":
		c, cerr := s.CommentRepo.Get(ctx, req.ItemId)
		if cerr != nil || c == nil {
			return nil, status.Error(codes.NotFound, "item not found")
		}
		// The comment must be on a public template (reporting private or
		// non-existent content is indistinguishable).
		tpl, terr := s.TemplateRepo.Get(ctx, c.TemplateID, userID)
		if terr != nil || tpl == nil || tpl.Visibility != "public" {
			return nil, status.Error(codes.NotFound, "item not found")
		}
		if c.UserID == userID {
			return nil, status.Error(codes.InvalidArgument, "you cannot report your own content")
		}
	}

	// Rate limit (FR-16).
	if retryAfter, ok := s.checkReportRate(ctx, userID); !ok {
		return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded, retry after %d seconds", retryAfter)
	}

	report := &models.ContentReport{
		ItemType:   req.ItemType,
		ItemID:     req.ItemId,
		ReporterID: userID,
		Reason:     req.Reason,
		Note:       note,
		Status:     "open",
	}
	if err := s.ReportRepo.Upsert(ctx, report); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create report: %v", err)
	}
	return reportToProto(report), nil
}

// GetMyReport returns the caller's open report on an item (drives the
// checkmark state), or an empty MyReport when none (FR-7).
func (s *PromptService) GetMyReport(ctx context.Context, req *pb.GetMyReportRequest) (*pb.MyReport, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if s.ReportRepo == nil {
		return nil, status.Error(codes.Unimplemented, "report repository not configured")
	}
	if req.ItemType != "template" && req.ItemType != "comment" {
		return nil, status.Error(codes.InvalidArgument, "item_type must be one of: template, comment")
	}
	rep, err := s.ReportRepo.GetMine(ctx, userID, req.ItemType, req.ItemId)
	if err != nil {
		if err == repository.ErrReportNotFound {
			return &pb.MyReport{}, nil
		}
		return nil, status.Errorf(codes.Internal, "failed to get report: %v", err)
	}
	return &pb.MyReport{Report: reportToProto(rep)}, nil
}

// WithdrawReport hard-deletes the caller's report on an item (FR-7).
func (s *PromptService) WithdrawReport(ctx context.Context, req *pb.WithdrawReportRequest) (*pb.WithdrawReportResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if s.ReportRepo == nil {
		return nil, status.Error(codes.Unimplemented, "report repository not configured")
	}
	if req.ItemType != "template" && req.ItemType != "comment" {
		return nil, status.Error(codes.InvalidArgument, "item_type must be one of: template, comment")
	}
	if err := s.ReportRepo.DeleteMine(ctx, userID, req.ItemType, req.ItemId); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to withdraw report: %v", err)
	}
	return &pb.WithdrawReportResponse{Success: true}, nil
}

func (s *PromptService) ListCategories(ctx context.Context, req *pb.ListCategoriesRequest) (*pb.ListCategoriesResponse, error) {
	zap.S().Infof("PromptService.ListCategories: owner_id=%s", req.OwnerId)
	filters := make(map[string]interface{})
	if req.OwnerId != "" {
		filters["owner_id"] = req.OwnerId
		filters["visibility"] = "private" // Assuming implicit private if owner specified for "My Prompts"
	}
	// If no owner, maybe public? Or sidebar expects all?
	// Sidebar "All Public" -> calls API.
	// Sidebar "My Prompts" -> calls API.
	// The sidebar logic implies it wants filtered stats.
	// Let's assume if owner_id is NOT provided, we return PUBLIC categories.
	if req.OwnerId == "" {
		filters["visibility"] = "public"
	}
	if req.Language != "" {
		filters["language"] = req.Language
	}

	stats, err := s.TemplateRepo.ListCategories(ctx, filters)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list categories: %v", err)
	}

	var pbStats []*pb.CategoryStats
	for _, s := range stats {
		pbStats = append(pbStats, &pb.CategoryStats{
			Name:  s.Name,
			Count: int32(s.Count),
		})
	}

	return &pb.ListCategoriesResponse{Categories: pbStats}, nil
}

func (s *PromptService) ListTags(ctx context.Context, req *pb.ListTagsRequest) (*pb.ListTagsResponse, error) {
	zap.S().Info("PromptService.ListTags")
	// Tags currently global in sidebar, so public only?
	filters := map[string]interface{}{
		"visibility": "public",
	}
	if req.Language != "" {
		filters["language"] = req.Language
	}
	stats, err := s.TemplateRepo.ListTags(ctx, filters)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list tags: %v", err)
	}

	var pbStats []*pb.TagStats
	for _, s := range stats {
		pbStats = append(pbStats, &pb.TagStats{
			Name:  s.Name,
			Count: int32(s.Count),
		})
	}

	return &pb.ListTagsResponse{Tags: pbStats}, nil
}

// --- Prompt RPCs ---

func (s *PromptService) CreatePrompt(ctx context.Context, req *pb.CreatePromptRequest) (*pb.CreatePromptResponse, error) {
	zap.S().Infof("PromptService.CreatePrompt: template_id=%s owner_id=%s", req.TemplateId, req.OwnerId)
	// Defense-in-depth: derive the owner from the authenticated context rather
	// than trusting the client-supplied owner_id.
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	req.OwnerId = userID

	variables := req.Variables
	if variables == nil {
		variables = make(map[string]string)
	}

	// Fetch template version content to render it
	version, err := s.TemplateVersionRepo.Get(ctx, req.VersionId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "template version not found")
	}

	// Visibility guard: a gated (in_review/rejected) template's content must
	// not be renderable through the prompt path by non-owners. The
	// repository Get returns 404-equivalent for hidden templates.
	if _, err := s.TemplateRepo.Get(ctx, version.TemplateID, userID); err != nil {
		return nil, status.Errorf(codes.NotFound, "template not found")
	}

	renderedContent := version.Content
	for k, v := range variables {
		renderedContent = strings.ReplaceAll(renderedContent, fmt.Sprintf("${%s}", k), v)
	}

	prompt := &models.Prompt{
		TemplateID: req.TemplateId,
		VersionID:  req.VersionId,
		OwnerID:    req.OwnerId,
		Variables:  variables,
		Content:    renderedContent,
	}

	if err := s.PromptRepo.Create(ctx, prompt); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create prompt: %v", err)
	}

	return &pb.CreatePromptResponse{
		Prompt: s.promptModelToProto(prompt),
	}, nil
}

func (s *PromptService) GetPrompt(ctx context.Context, req *pb.GetPromptRequest) (*pb.GetPromptResponse, error) {
	zap.S().Infof("PromptService.GetPrompt: id=%s", req.Id)
	prompt, err := s.PromptRepo.Get(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "prompt not found")
	}
	return &pb.GetPromptResponse{Prompt: s.promptModelToProto(prompt)}, nil
}

func (s *PromptService) ListPrompts(ctx context.Context, req *pb.ListPromptsRequest) (*pb.ListPromptsResponse, error) {
	zap.S().Infof("PromptService.ListPrompts: owner_id=%s template_id=%s page_size=%d", req.OwnerId, req.TemplateId, req.PageSize)
	// AuthZ: listing another user's prompts (which contain private variable
	// values) requires authentication and must be limited to the caller's own
	// prompts (or an admin).
	if req.OwnerId != "" {
		userID, err := GetUserIDFromContext(ctx)
		if err != nil {
			return nil, err
		}
		if req.OwnerId != userID && GetUserRoleFromContext(ctx) != "admin" {
			return nil, status.Errorf(codes.PermissionDenied, "not authorized")
		}
	}
	limit := int(req.PageSize)
	if limit <= 0 {
		limit = 10
	}
	offset := 0
	if req.PageToken != "" {
		if v, err := strconv.Atoi(req.PageToken); err == nil {
			offset = v
		}
	}

	filters := make(map[string]interface{})
	if req.OwnerId != "" {
		filters["owner_id"] = req.OwnerId
	}
	if req.TemplateId != "" {
		// Prompts carry rendered content — hidden (in_review/rejected)
		// templates expose no prompts to non-owners (constraint 5).
		userID, _ := GetUserIDFromContext(ctx)
		if _, terr := s.TemplateRepo.Get(ctx, req.TemplateId, userID); terr != nil {
			return nil, status.Errorf(codes.NotFound, "template not found")
		}
		filters["template_id"] = req.TemplateId
	}

	prompts, err := s.PromptRepo.List(ctx, limit, offset, filters)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list prompts: %v", err)
	}

	var pbPrompts []*pb.Prompt
	for _, p := range prompts {
		pbPrompts = append(pbPrompts, s.promptModelToProto(p))
	}

	nextPageToken := ""
	if len(prompts) == limit {
		nextPageToken = strconv.Itoa(offset + limit)
	}

	return &pb.ListPromptsResponse{Prompts: pbPrompts, NextPageToken: nextPageToken}, nil
}

// ListTemplateVersions lists all versions of a template.
func (s *PromptService) ListTemplateVersions(ctx context.Context, req *pb.ListTemplateVersionsRequest) (*pb.ListTemplateVersionsResponse, error) {
	zap.S().Infof("PromptService.ListTemplateVersions: template_id=%s page_size=%d", req.TemplateId, req.PageSize)
	if req.TemplateId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "template_id is required")
	}

	// Hidden (in_review/rejected) templates expose no version history to
	// non-owners — 404 before listing (constraint 5).
	userID, _ := GetUserIDFromContext(ctx)
	if _, err := s.TemplateRepo.Get(ctx, req.TemplateId, userID); err != nil {
		return nil, status.Errorf(codes.NotFound, "template not found")
	}

	limit := int(req.PageSize)
	if limit <= 0 {
		limit = 10
	}
	offset := 0
	if req.PageToken != "" {
		if v, err := strconv.Atoi(req.PageToken); err == nil {
			offset = v
		}
	}

	versions, err := s.TemplateVersionRepo.List(ctx, limit, offset, req.TemplateId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list template versions: %v", err)
	}

	var pbVersions []*pb.TemplateVersion
	for _, v := range versions {
		pbVersions = append(pbVersions, s.versionModelToProto(v))
	}

	nextPageToken := ""
	if len(versions) == limit {
		nextPageToken = strconv.Itoa(offset + limit)
	}

	return &pb.ListTemplateVersionsResponse{Versions: pbVersions, NextPageToken: nextPageToken}, nil
}

func (s *PromptService) DeletePrompt(ctx context.Context, req *pb.DeletePromptRequest) (*pb.DeletePromptResponse, error) {
	zap.S().Infof("PromptService.DeletePrompt: id=%s owner_id=%s", req.Id, req.OwnerId)
	prompt, err := s.PromptRepo.Get(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "prompt not found")
	}

	if err := authorizeOwnerOrAdmin(ctx, prompt.OwnerID); err != nil {
		return nil, err
	}

	if err := s.PromptRepo.Delete(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete prompt: %v", err)
	}

	return &pb.DeletePromptResponse{Success: true}, nil
}

// --- Helpers ---

func (s *PromptService) templateModelToProto(m *models.Template) *pb.Template {
	if m == nil {
		return nil
	}
	vis := pb.Visibility_VISIBILITY_PRIVATE
	if m.Visibility == "public" {
		vis = pb.Visibility_VISIBILITY_PUBLIC
	}

	tpl := &pb.Template{
		Id:            m.ID,
		OwnerId:       m.OwnerID,
		Title:         m.Title,
		Description:   m.Description.String,
		Visibility:    vis,
		Tags:          m.Tags,
		Category:      m.Category.String,
		CreatedAt:     timestamppb.New(m.CreatedAt),
		UpdatedAt:     timestamppb.New(m.UpdatedAt),
		LikeCount:     m.LikeCount,
		FavoriteCount: m.FavoriteCount,
		CommentCount:  m.CommentCount,
		ViewCount:     m.ViewCount,
		UseCount:      m.UseCount,
		IsLiked:       m.IsLiked,
		IsFavorited:   m.IsFavorited,
		Language:      m.Language,
		RatingCount:   m.RatingCount,
		MyRating:      m.MyRating,
	}
	// The average is always computed, never stored; guard division by zero.
	if m.RatingCount > 0 {
		tpl.RatingAvg = float64(m.RatingSum) / float64(m.RatingCount)
	}
	// Moderation state: default to "normal" for legacy rows / zero values so
	// the field is always meaningful to clients.
	moderationStatus := m.ModerationStatus
	if moderationStatus == "" {
		moderationStatus = "normal"
	}
	tpl.ModerationStatus = moderationStatus
	if m.PublishedAt.Valid {
		tpl.PublishedAt = timestamppb.New(m.PublishedAt.Time)
	}
	tpl.ContentWarnings = m.ContentWarnings
	tpl.LockedWarnings = m.LockedWarnings
	tpl.OwnerDisplayName = m.OwnerDisplayName
	tpl.OwnerAvatar = m.OwnerAvatar
	tpl.ModerationReason = m.ModerationReason
	return tpl
}

// clearModerationReasonForNonOwner blanks moderation_reason unless the viewer
// is the template owner. The reason is owner-only (like moderation_status) and
// must not leak to anonymous/non-owner viewers (e.g. a restored template keeps
// its restore reason while public).
func clearModerationReasonForNonOwner(tpl *pb.Template, viewerID string) {
	if tpl == nil {
		return
	}
	if viewerID == "" || viewerID != tpl.OwnerId {
		tpl.ModerationReason = ""
	}
}

func (s *PromptService) versionModelToProto(m *models.TemplateVersion) *pb.TemplateVersion {
	if m == nil {
		return nil
	}
	changeNote := ""
	if m.ChangeNote.Valid {
		changeNote = m.ChangeNote.String
	}
	return &pb.TemplateVersion{
		Id:          m.ID,
		TemplateId:  m.TemplateID,
		Version:     m.Version,
		Content:     m.Content,
		ChangeNote:  changeNote,
		PromptCount: int32(m.PromptCount),
		CreatedAt:   timestamppb.New(m.CreatedAt),
	}
}

func (s *PromptService) promptModelToProto(m *models.Prompt) *pb.Prompt {
	if m == nil {
		return nil
	}
	variables := m.Variables
	if variables == nil {
		variables = make(map[string]string)
	}

	// Dynamically resolve variables in the template content
	rendered := m.Content
	for k, v := range variables {
		// e.g., Replace ${variable} with value
		rendered = strings.ReplaceAll(rendered, "${"+k+"}", v)
	}

	return &pb.Prompt{
		Id:            m.ID,
		TemplateId:    m.TemplateID,
		VersionId:     m.VersionID,
		OwnerId:       m.OwnerID,
		Variables:     variables,
		Content:       rendered,
		CreatedAt:     timestamppb.New(m.CreatedAt),
		TemplateTitle: m.TemplateTitle,
	}
}

// --- Alias RPCs ---

func (s *PromptService) aliasModelToProto(m *models.TemplateAlias) *pb.Alias {
	if m == nil {
		return nil
	}
	return &pb.Alias{
		Id:         m.ID,
		TemplateId: m.TemplateID,
		AliasName:  m.AliasName,
		VersionId:  m.VersionID,
	}
}

// authorizeOwnerOrAdmin verifies the caller (from context) is the owner of the
// resource or an admin. It never trusts client-supplied owner identifiers.
func authorizeOwnerOrAdmin(ctx context.Context, ownerID string) error {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	if userID != ownerID && GetUserRoleFromContext(ctx) != "admin" {
		return status.Errorf(codes.PermissionDenied, "not authorized")
	}
	return nil
}

// authorizeAliasMutation verifies the caller owns the template an alias
// operation targets (or is an admin). Fetch with the caller's id so owners
// can manage aliases on their in_review/rejected templates (FR-3/FR-6).
func (s *PromptService) authorizeAliasMutation(ctx context.Context, templateID string) error {
	userID, _ := GetUserIDFromContext(ctx)
	template, err := s.TemplateRepo.Get(ctx, templateID, userID)
	if err != nil || template == nil {
		return status.Errorf(codes.NotFound, "template not found")
	}
	return authorizeOwnerOrAdmin(ctx, template.OwnerID)
}

func (s *PromptService) CreateAlias(ctx context.Context, req *pb.CreateAliasRequest) (*pb.Alias, error) {
	if err := s.authorizeAliasMutation(ctx, req.TemplateId); err != nil {
		return nil, err
	}
	alias := &models.TemplateAlias{
		TemplateID: req.TemplateId,
		AliasName:  req.AliasName,
		VersionID:  req.VersionId,
	}
	if err := s.TemplateAliasRepo.Create(ctx, alias); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create alias: %v", err)
	}
	return s.aliasModelToProto(alias), nil
}

func (s *PromptService) UpdateAlias(ctx context.Context, req *pb.UpdateAliasRequest) (*pb.Alias, error) {
	if err := s.authorizeAliasMutation(ctx, req.TemplateId); err != nil {
		return nil, err
	}
	alias := &models.TemplateAlias{
		TemplateID: req.TemplateId,
		AliasName:  req.AliasName,
		VersionID:  req.VersionId,
	}
	if err := s.TemplateAliasRepo.Update(ctx, alias); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update alias: %v", err)
	}

	// Get it back to return full object
	updatedAlias, err := s.TemplateAliasRepo.Get(ctx, req.TemplateId, req.AliasName)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "alias not found after update")
	}
	return s.aliasModelToProto(updatedAlias), nil
}

func (s *PromptService) ListAliases(ctx context.Context, req *pb.ListAliasesRequest) (*pb.ListAliasesResponse, error) {
	aliases, err := s.TemplateAliasRepo.List(ctx, req.TemplateId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list aliases: %v", err)
	}

	var pbAliases []*pb.Alias
	hasLatest := false
	for _, a := range aliases {
		if a.AliasName == "latest" {
			hasLatest = true
		}
		pbAliases = append(pbAliases, s.aliasModelToProto(a))
	}

	if !hasLatest {
		latestVersion, err := s.TemplateVersionRepo.GetLatest(ctx, req.TemplateId)
		if err == nil && latestVersion != nil {
			pbAliases = append(pbAliases, &pb.Alias{
				TemplateId: req.TemplateId,
				AliasName:  "latest",
				VersionId:  int32(latestVersion.ID),
			})
			_ = s.TemplateAliasRepo.Upsert(ctx, &models.TemplateAlias{
				TemplateID: req.TemplateId,
				AliasName:  "latest",
				VersionID:  latestVersion.ID,
			})
		}
	}

	return &pb.ListAliasesResponse{
		Aliases: pbAliases,
	}, nil
}

func (s *PromptService) DeleteAlias(ctx context.Context, req *pb.DeleteAliasRequest) (*pb.DeletePromptResponse, error) {
	if err := s.authorizeAliasMutation(ctx, req.TemplateId); err != nil {
		return nil, err
	}
	if err := s.TemplateAliasRepo.Delete(ctx, req.TemplateId, req.AliasName); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete alias: %v", err)
	}
	return &pb.DeletePromptResponse{Success: true}, nil
}

func (s *PromptService) GetPromptByAlias(ctx context.Context, req *pb.GetPromptByAliasRequest) (*pb.TemplateVersion, error) {
	alias, err := s.TemplateAliasRepo.Get(ctx, req.TemplateId, req.AliasName)
	if err != nil {
		if req.AliasName == "latest" {
			latestVersion, lErr := s.TemplateVersionRepo.GetLatest(ctx, req.TemplateId)
			if lErr == nil && latestVersion != nil {
				alias = &models.TemplateAlias{
					TemplateID: req.TemplateId,
					AliasName:  "latest",
					VersionID:  latestVersion.ID,
				}
			} else {
				return nil, status.Errorf(codes.NotFound, "alias not found (no latest version)")
			}
		} else {
			return nil, status.Errorf(codes.NotFound, "alias not found")
		}
	}

	// Hidden (in_review/rejected) templates expose no content through the
	// alias path — 404 for non-owners (constraint 5).
	userID, _ := GetUserIDFromContext(ctx)
	if _, terr := s.TemplateRepo.Get(ctx, alias.TemplateID, userID); terr != nil {
		return nil, status.Errorf(codes.NotFound, "template not found")
	}

	version, err := s.TemplateVersionRepo.Get(ctx, alias.VersionID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "version not found")
	}

	// Extract variables using regexp
	re := regexp.MustCompile(`\$\$(.*?)\$\$`)
	matches := re.FindAllStringSubmatch(version.Content, -1)
	vars := []string{}
	seen := map[string]bool{}
	for _, m := range matches {
		if len(m) > 1 {
			v := m[1]
			if !seen[v] {
				_ = append(vars, v)
				seen[v] = true
			}
		}
	}

	return s.versionModelToProto(version), nil
}
