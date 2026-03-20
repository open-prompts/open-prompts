package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
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
	PromptRepo          repository.PromptRepository
	TemplateRepo        repository.TemplateRepository
	TemplateVersionRepo repository.TemplateVersionRepository
	TemplateAliasRepo   repository.TemplateAliasRepository
}

func NewPromptService(
	promptRepo repository.PromptRepository,
	templateRepo repository.TemplateRepository,
	templateVersionRepo repository.TemplateVersionRepository,
	templateAliasRepo repository.TemplateAliasRepository,
) *PromptService {
	return &PromptService{
		PromptRepo:          promptRepo,
		TemplateRepo:        templateRepo,
		TemplateVersionRepo: templateVersionRepo,
		TemplateAliasRepo:   templateAliasRepo,
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

	template := &models.Template{
		OwnerID:     userID, // Use the authenticated user ID
		Title:       req.Title,
		Description: sql.NullString{String: req.Description, Valid: req.Description != ""},
		Visibility:  visibility,
		Type:        typeStr,
		Tags:        pq.StringArray(req.Tags),
		Category:    sql.NullString{String: req.Category, Valid: req.Category != ""},
		Language:    language,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
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
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
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

	return &pb.CreateTemplateResponse{
		Template: s.templateModelToProto(newTpl),
		Version:  s.versionModelToProto(newVer),
	}, nil
}

// UpdateTemplate updates an existing template.
func (s *PromptService) UpdateTemplate(ctx context.Context, req *pb.UpdateTemplateRequest) (*pb.UpdateTemplateResponse, error) {
	zap.S().Infof("PromptService.UpdateTemplate: template_id=%s", req.TemplateId)
	// Get existing template
	template, err := s.TemplateRepo.Get(ctx, req.TemplateId, "")
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "template not found")
	}

	// AuthZ
	if req.OwnerId != "" && template.OwnerID != req.OwnerId {
		return nil, status.Errorf(codes.PermissionDenied, "not authorized")
	}

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

	template.UpdatedAt = time.Now()

	if err := s.TemplateRepo.Update(ctx, template); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update template: %v", err)
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

	return &pb.GetTemplateResponse{
		Template:      s.templateModelToProto(template),
		LatestVersion: s.versionModelToProto(latest),
	}, nil
}

// ListTemplates retrieves a list of templates.
func (s *PromptService) ListTemplates(ctx context.Context, req *pb.ListTemplatesRequest) (*pb.ListTemplatesResponse, error) {
	userID, _ := GetUserIDFromContext(ctx)
	zap.S().Infof("PromptService.ListTemplates: page_size=%d page_token=%s owner_id=%s visibility=%s user_id=%s", req.PageSize, req.PageToken, req.OwnerId, req.Visibility, userID)

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

		templates, nextToken, err := fetch(limit, offset, filters)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to list templates: %v", err)
		}
		return &pb.ListTemplatesResponse{Templates: templates, NextPageToken: nextToken}, nil
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

		templates, nextToken, err := fetch(limit, offset, filters)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to list templates: %v", err)
		}
		return &pb.ListTemplatesResponse{Templates: templates, NextPageToken: nextToken}, nil
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

		templates, nextToken, err := fetch(limit, offset, filters)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to list templates: %v", err)
		}
		return &pb.ListTemplatesResponse{Templates: templates, NextPageToken: nextToken}, nil
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

	return &pb.ListTemplatesResponse{
		Templates:            publicTemplates,
		NextPageToken:        nextPublicToken,
		PrivateTemplates:     privateTemplates,
		PrivateNextPageToken: nextPrivateToken,
	}, nil
}

func (s *PromptService) DeleteTemplate(ctx context.Context, req *pb.DeleteTemplateRequest) (*pb.DeleteTemplateResponse, error) {
	zap.S().Infof("PromptService.DeleteTemplate: id=%s owner_id=%s", req.Id, req.OwnerId)
	template, err := s.TemplateRepo.Get(ctx, req.Id, "")
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "template not found")
	}

	if req.OwnerId != "" && template.OwnerID != req.OwnerId {
		return nil, status.Errorf(codes.PermissionDenied, "not authorized")
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

	isLiked, count, err := s.TemplateRepo.ToggleLike(ctx, userID, req.TemplateId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to toggle like: %v", err)
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

	isFavorited, count, err := s.TemplateRepo.ToggleFavorite(ctx, userID, req.TemplateId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to toggle favorite: %v", err)
	}

	return &pb.ToggleFavoriteResponse{
		IsFavorited:   isFavorited,
		FavoriteCount: count,
	}, nil
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
	if req.OwnerId == "" {
		return nil, status.Error(codes.InvalidArgument, "owner_id is required")
	}

	variablesJSON, err := json.Marshal(req.Variables)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid variables: %v", err)
	}

	prompt := &models.Prompt{
		TemplateID: req.TemplateId,
		VersionID:  req.VersionId,
		OwnerID:    req.OwnerId,
		Variables:  variablesJSON,
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

	if req.OwnerId != "" && prompt.OwnerID != req.OwnerId {
		return nil, status.Errorf(codes.PermissionDenied, "not authorized")
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

	return &pb.Template{
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
		IsLiked:       m.IsLiked,
		IsFavorited:   m.IsFavorited,
		Language:      m.Language,
	}
}

func (s *PromptService) versionModelToProto(m *models.TemplateVersion) *pb.TemplateVersion {
	if m == nil {
		return nil
	}
	return &pb.TemplateVersion{
		Id:         m.ID,
		TemplateId: m.TemplateID,
		Version:    m.Version,
		Content:    m.Content,
		CreatedAt:  timestamppb.New(m.CreatedAt),
	}
}

func (s *PromptService) promptModelToProto(m *models.Prompt) *pb.Prompt {
	if m == nil {
		return nil
	}
	var variables []string
	// Prefer parsing as array of objects (new format), fallback to array of strings (legacy)
	var objs []map[string]interface{}
	if err := json.Unmarshal(m.Variables, &objs); err == nil {
		for _, obj := range objs {
			for k, v := range obj {
				variables = append(variables, fmt.Sprintf("%s:%v", k, v))
				break
			}
		}
	} else {
		_ = json.Unmarshal(m.Variables, &variables)
	}

	return &pb.Prompt{
		Id:         m.ID,
		TemplateId: m.TemplateID,
		VersionId:  m.VersionID,
		OwnerId:    m.OwnerID,
		Variables:  variables,
		CreatedAt:  timestamppb.New(m.CreatedAt),
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

func (s *PromptService) CreateAlias(ctx context.Context, req *pb.CreateAliasRequest) (*pb.Alias, error) {
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
