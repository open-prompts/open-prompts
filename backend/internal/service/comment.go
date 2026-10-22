package service

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	commentMaxLen        = 2000
	commentEditWindow    = 15 * time.Minute
	commentRatePerMinDef = 5
	commentRatePerDayDef = 50
	commentThreadCap     = 50
)

// ListComments lists comments for a template. Visibility follows the
// template: public templates are anonymously readable, private ones only by
// the owner (indistinguishable from missing).
func (s *PromptService) ListComments(ctx context.Context, req *pb.ListCommentsRequest) (*pb.ListCommentsResponse, error) {
	userID, _ := GetUserIDFromContext(ctx) // optional auth

	tpl, err := s.TemplateRepo.Get(ctx, req.TemplateId, userID)
	if err != nil {
		return nil, status.Error(codes.NotFound, "template not found")
	}
	if tpl.Visibility == "private" && tpl.OwnerID != userID {
		return nil, status.Error(codes.NotFound, "template not found") // indistinguishable
	}

	page := int(req.Page)
	if page < 1 {
		page = 1
	}
	size := int(req.PageSize)
	if size < 1 {
		size = 20
	}
	if size > 50 {
		size = 50
	}

	comments, total, err := s.CommentRepo.List(ctx, tpl.ID, size, (page-1)*size)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to list comments")
	}

	resp := &pb.ListCommentsResponse{
		Comments:   make([]*pb.Comment, 0, len(comments)),
		TotalCount: int32(total),
		Page:       int32(page),
		PageSize:   int32(size),
	}
	for _, c := range comments {
		resp.Comments = append(resp.Comments, s.commentModelToProto(c, tpl, userID))
	}
	return resp, nil
}

// CreateComment posts a comment on a template (auth required, banned users
// rejected, rate limited per user).
func (s *PromptService) CreateComment(ctx context.Context, req *pb.CreateCommentRequest) (*pb.Comment, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	content := strings.TrimSpace(req.Content)
	if content == "" || len([]rune(content)) > commentMaxLen {
		return nil, status.Error(codes.InvalidArgument, "content must be 1-2000 characters")
	}

	user, err := s.UserRepo.GetByID(ctx, userID)
	if err != nil || user == nil {
		return nil, status.Error(codes.Unauthenticated, "user not found")
	}
	if user.Status == "banned" {
		return nil, status.Error(codes.PermissionDenied, "banned users cannot comment")
	}

	tpl, err := s.TemplateRepo.Get(ctx, req.TemplateId, userID)
	if err != nil {
		return nil, status.Error(codes.NotFound, "template not found")
	}
	if tpl.Visibility == "private" && tpl.OwnerID != userID {
		return nil, status.Error(codes.NotFound, "template not found") // indistinguishable
	}

	// Resolve the parent (if any). Must be a top-level comment on the same
	// template; replying to a reply is flattened onto the top-level thread.
	parentID := strings.TrimSpace(req.ParentCommentId)
	if parentID != "" {
		parent, err := s.CommentRepo.Get(ctx, parentID)
		if err == sql.ErrNoRows {
			return nil, status.Error(codes.InvalidArgument, "parent comment not found")
		}
		if err != nil {
			return nil, status.Error(codes.Internal, "failed to load parent comment")
		}
		if parent.TemplateID != tpl.ID {
			return nil, status.Error(codes.InvalidArgument, "parent comment is on a different template")
		}
		if parent.ParentCommentID != "" {
			return nil, status.Error(codes.InvalidArgument, "parent must be a top-level comment")
		}
		if parent.ReplyCount >= commentThreadCap {
			return nil, status.Error(codes.InvalidArgument, "thread is full — post a new comment instead")
		}
	}

	if retryAfter, ok := s.checkCommentRate(ctx, userID); !ok {
		return nil, status.Error(codes.ResourceExhausted,
			fmt.Sprintf("rate limit exceeded, retry after %d seconds", retryAfter))
	}

	c := &models.Comment{
		TemplateID: tpl.ID, UserID: userID, Content: content,
		AuthorDisplayName: user.DisplayName, AuthorAvatar: user.Avatar,
		ParentCommentID: parentID,
	}
	if err := s.CommentRepo.Create(ctx, c); err != nil {
		return nil, status.Error(codes.Internal, "failed to create comment")
	}

	// FR-17: best-effort owner notification, after the write commits.
	s.createEngagementNotification(ctx, "comment", tpl.ID, tpl.OwnerID, userID)

	// Notify the parent comment's author (not the template owner) when this
	// is a reply. Self-replies are suppressed inside notifyCommentReply.
	if parentID != "" {
		s.notifyCommentReply(ctx, tpl, parentID, userID, content)
	}

	return s.commentModelToProto(c, tpl, userID), nil
}

// UpdateComment edits the author's own comment within the edit window.
func (s *PromptService) UpdateComment(ctx context.Context, req *pb.UpdateCommentRequest) (*pb.Comment, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	content := strings.TrimSpace(req.Content)
	if content == "" || len([]rune(content)) > commentMaxLen {
		return nil, status.Error(codes.InvalidArgument, "content must be 1-2000 characters")
	}

	c, err := s.CommentRepo.Get(ctx, req.CommentId)
	if err == sql.ErrNoRows {
		return nil, status.Error(codes.NotFound, "comment not found")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load comment")
	}
	if c.UserID != userID {
		return nil, status.Error(codes.PermissionDenied, "only the author can edit a comment")
	}

	user, err := s.UserRepo.GetByID(ctx, userID)
	if err != nil || user == nil {
		return nil, status.Error(codes.Unauthenticated, "user not found")
	}
	if user.Status == "banned" {
		return nil, status.Error(codes.PermissionDenied, "banned users cannot comment")
	}

	if time.Since(c.CreatedAt) > commentEditWindow {
		return nil, status.Error(codes.PermissionDenied, "editable window expired — delete and repost instead")
	}

	updated, err := s.CommentRepo.Update(ctx, req.CommentId, content)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to update comment")
	}

	tpl, err := s.TemplateRepo.Get(ctx, updated.TemplateID, userID)
	if err != nil {
		return nil, status.Error(codes.NotFound, "template not found")
	}
	return s.commentModelToProto(updated, tpl, userID), nil
}

// DeleteComment deletes a comment. The caller must be the comment author or
// the template owner (FR-10). Un-audited by design.
func (s *PromptService) DeleteComment(ctx context.Context, req *pb.DeleteCommentRequest) (*pb.DeleteCommentResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	c, err := s.CommentRepo.Get(ctx, req.CommentId)
	if err == sql.ErrNoRows {
		return nil, status.Error(codes.NotFound, "comment not found")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load comment")
	}

	tpl, err := s.TemplateRepo.Get(ctx, c.TemplateID, userID)
	if err != nil {
		return nil, status.Error(codes.NotFound, "template not found")
	}

	if c.UserID != userID && tpl.OwnerID != userID {
		return nil, status.Error(codes.PermissionDenied, "only the comment author or template owner can delete a comment")
	}

	deleted, _, err := s.CommentRepo.Delete(ctx, req.CommentId)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to delete comment")
	}
	if !deleted {
		return nil, status.Error(codes.NotFound, "comment not found")
	}

	// Auto-resolve the comment's open reports as actioned (FR-12).
	if s.ReportRepo != nil {
		if aerr := s.ReportRepo.AutoResolveItem(ctx, "comment", req.CommentId, userID); aerr != nil {
			zap.S().Errorf("auto-resolve reports on comment delete: failed (comment=%s): %v", req.CommentId, aerr)
		}
	}
	return &pb.DeleteCommentResponse{Success: true}, nil
}

// ListCommentReplies lists all replies to a top-level comment, oldest-first,
// unpaginated (bounded by the 50-reply thread cap). Visibility follows the
// template: public templates are anonymously readable, private ones only by
// the owner (indistinguishable from missing).
func (s *PromptService) ListCommentReplies(ctx context.Context, req *pb.ListCommentRepliesRequest) (*pb.ListCommentRepliesResponse, error) {
	userID, _ := GetUserIDFromContext(ctx) // optional auth

	parent, err := s.CommentRepo.Get(ctx, req.CommentId)
	if err == sql.ErrNoRows {
		return nil, status.Error(codes.NotFound, "comment not found")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load comment")
	}
	if parent.ParentCommentID != "" {
		return nil, status.Error(codes.InvalidArgument, "replies are only available for top-level comments")
	}

	tpl, err := s.TemplateRepo.Get(ctx, parent.TemplateID, userID)
	if err != nil {
		return nil, status.Error(codes.NotFound, "template not found")
	}
	if tpl.Visibility == "private" && tpl.OwnerID != userID {
		return nil, status.Error(codes.NotFound, "template not found") // indistinguishable
	}

	replies, err := s.CommentRepo.ListReplies(ctx, parent.ID)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to list replies")
	}
	resp := &pb.ListCommentRepliesResponse{Replies: make([]*pb.Comment, 0, len(replies))}
	for _, r := range replies {
		resp.Replies = append(resp.Replies, s.commentModelToProto(r, tpl, userID))
	}
	return resp, nil
}

// notifyCommentReply emits a comment_reply notification to the parent
// comment's author (not the template owner). It is strictly best-effort:
// self-replies are suppressed and any failure is logged and swallowed so the
// triggering reply is never affected. The snippet is the reply's first 100
// runes.
func (s *PromptService) notifyCommentReply(ctx context.Context, tpl *models.Template, parentID, actorID, content string) {
	if s.NotificationRepo == nil || s.UserRepo == nil {
		return
	}
	parent, err := s.CommentRepo.Get(ctx, parentID)
	if err != nil || parent == nil {
		zap.S().Warnf("comment_reply hook: failed to load parent %s: %v", parentID, err)
		return
	}
	if parent.UserID == actorID {
		return // replying to your own comment sends no notification
	}
	actor, err := s.UserRepo.GetByID(ctx, actorID)
	if err != nil || actor == nil {
		zap.S().Warnf("comment_reply hook: failed to load actor %s: %v", actorID, err)
		return
	}
	displayName := actor.DisplayName
	if displayName == "" {
		displayName = actor.ID
	}
	snippet := content
	if r := []rune(snippet); len(r) > 100 {
		snippet = string(r[:100])
	}
	if err := s.NotificationRepo.Create(ctx, &models.Notification{
		RecipientID:      parent.UserID,
		ActorID:          actorID,
		TemplateID:       tpl.ID,
		Type:             "comment_reply",
		ActorDisplayName: displayName,
		Snippet:          snippet,
	}); err != nil {
		zap.S().Errorf("comment_reply hook: failed to create notification (template=%s parent=%s actor=%s): %v",
			tpl.ID, parentID, actorID, err)
	}
}

// commentModelToProto maps a comment row to its proto representation,
// computing is_edited / is_owner / is_own for the caller.
func (s *PromptService) commentModelToProto(c *models.Comment, tpl *models.Template, viewerID string) *pb.Comment {
	if c == nil {
		return nil
	}
	return &pb.Comment{
		Id:         c.ID,
		TemplateId: c.TemplateID,
		Content:    c.Content,
		CreatedAt:  timestamppb.New(c.CreatedAt),
		UpdatedAt:  timestamppb.New(c.UpdatedAt),
		IsEdited:   c.UpdatedAt.After(c.CreatedAt),
		Author: &pb.CommentAuthor{
			Id:          c.UserID,
			DisplayName: c.AuthorDisplayName,
			Avatar:      c.AuthorAvatar,
		},
		IsOwner:         tpl != nil && c.UserID == tpl.OwnerID,
		IsOwn:           c.UserID == viewerID,
		ReplyCount:      c.ReplyCount,
		HasCreatorReply: c.HasCreatorReply,
	}
}

// checkCommentRate enforces COMMENT_RATE_PER_MINUTE and COMMENT_RATE_PER_DAY
// using fixed-window counters in Redis. Returns (retryAfterSeconds, false)
// when over limit. Fail-open when Redis is unavailable (s.Redis == nil or
// Get/Set errors), matching the admin throttle. Only successful creates
// consume quota: the caller invokes this right before the insert.
func (s *PromptService) checkCommentRate(ctx context.Context, userID string) (int, bool) {
	if s.Redis == nil {
		return 0, true
	}
	limits := []struct {
		key    string
		max    string
		window time.Duration
	}{
		{"comment_rate_min:" + userID, os.Getenv("COMMENT_RATE_PER_MINUTE"), time.Minute},
		{"comment_rate_day:" + userID, os.Getenv("COMMENT_RATE_PER_DAY"), 24 * time.Hour},
	}
	minRetry := 0
	for _, l := range limits {
		max, _ := strconv.Atoi(l.max)
		if max <= 0 {
			if l.window == time.Minute {
				max = commentRatePerMinDef
			} else {
				max = commentRatePerDayDef
			}
		}
		v, err := s.Redis.Get(ctx, l.key)
		if err != nil || v == "" {
			_ = s.Redis.Set(ctx, l.key, "1:"+strconv.FormatInt(time.Now().Add(l.window).Unix(), 10), l.window)
			continue
		}
		var count int
		var expires int64
		if _, err := fmt.Sscanf(v, "%d:%d", &count, &expires); err != nil {
			_ = s.Redis.Set(ctx, l.key, "1:"+strconv.FormatInt(time.Now().Add(l.window).Unix(), 10), l.window)
			continue
		}
		if time.Now().Unix() >= expires { // window elapsed: reset
			_ = s.Redis.Set(ctx, l.key, "1:"+strconv.FormatInt(time.Now().Add(l.window).Unix(), 10), l.window)
			continue
		}
		if count >= max {
			if r := int(expires - time.Now().Unix()); r > minRetry {
				minRetry = r
			}
			continue
		}
		_ = s.Redis.Set(ctx, l.key,
			fmt.Sprintf("%d:%d", count+1, expires),
			time.Duration(expires-time.Now().Unix())*time.Second)
	}
	if minRetry > 0 {
		return minRetry, false
	}
	return 0, true
}
