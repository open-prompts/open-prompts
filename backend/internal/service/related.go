package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// relatedCacheTTL bounds the staleness of the related rail cache. Like
	// counts may lag within the TTL; determinism (NFR-3) is preserved.
	relatedCacheTTL = 5 * time.Minute
	// relatedDefaultLimit is the default number of related templates.
	relatedDefaultLimit = 8
	// authorDefaultLimit is the default number of "more from author" items.
	authorDefaultLimit = 4
)

// clampInt clamps v into [lo, hi]. If v is 0 (unset), it returns def.
func clampInt(v, def, lo, hi int) int {
	if v == 0 {
		v = def
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// GetRelatedTemplates returns explainable related templates for a public
// template. Private templates 404 for everyone (including the owner) — rails
// are a public-discovery surface. Results are cached in Redis for 5 minutes;
// cache failures are logged and swallowed (rails are an enhancement).
func (s *PromptService) GetRelatedTemplates(ctx context.Context, req *pb.GetRelatedTemplatesRequest) (*pb.GetRelatedTemplatesResponse, error) {
	userID, _ := GetUserIDFromContext(ctx) // optional auth

	tpl, err := s.TemplateRepo.Get(ctx, req.TemplateId, userID)
	if err != nil {
		return nil, status.Error(codes.NotFound, "template not found")
	}
	if tpl.Visibility != "public" {
		return nil, status.Error(codes.NotFound, "template not found")
	}

	limit := clampInt(int(req.Limit), relatedDefaultLimit, 3, 8)

	key := fmt.Sprintf("related:v1:%s:%d", tpl.ID, limit)
	if cached, err := s.Redis.Get(ctx, key); err == nil {
		var resp pb.GetRelatedTemplatesResponse
		if json.Unmarshal([]byte(cached), &resp) == nil {
			return &resp, nil
		}
		zap.S().Warnf("GetRelatedTemplates: failed to unmarshal cache for %s", key)
	} else {
		zap.S().Debugf("GetRelatedTemplates: cache miss for %s", key)
	}

	items, err := s.TemplateRepo.ListRelated(ctx, tpl.ID, tpl, userID, limit)
	if err != nil {
		zap.S().Errorf("GetRelatedTemplates: error listing related for %s: %v", tpl.ID, err)
		return nil, status.Error(codes.Internal, "failed to list related templates")
	}

	resp := &pb.GetRelatedTemplatesResponse{
		Items: make([]*pb.Template, 0, len(items)),
		Basis: "generic",
	}
	for _, it := range items {
		pbT := s.templateModelToProto(it)
		clearModerationReasonForNonOwner(pbT, userID)
		resp.Items = append(resp.Items, pbT)
	}
	resp.Basis = deriveBasis(tpl, items)

	// Fewer than 3 survivors -> empty items (never an error); the frontend
	// hides the section.
	if len(resp.Items) < 3 {
		resp.Items = []*pb.Template{}
		resp.Basis = "generic"
	}

	if b, err := json.Marshal(resp); err == nil {
		if err := s.Redis.Set(ctx, key, string(b), relatedCacheTTL); err != nil {
			zap.S().Warnf("GetRelatedTemplates: cache write failed for %s: %v", key, err)
		}
	} else {
		zap.S().Warnf("GetRelatedTemplates: cache marshal failed for %s: %v", key, err)
	}

	return resp, nil
}

// ListMoreFromAuthor returns the author's other public templates, most-liked
// first. Private templates 404 for everyone (including the owner). Not cached
// — the author row is a single indexed owner_id lookup with LIMIT 4.
func (s *PromptService) ListMoreFromAuthor(ctx context.Context, req *pb.ListMoreFromAuthorRequest) (*pb.ListMoreFromAuthorResponse, error) {
	userID, _ := GetUserIDFromContext(ctx) // optional auth

	tpl, err := s.TemplateRepo.Get(ctx, req.TemplateId, userID)
	if err != nil {
		return nil, status.Error(codes.NotFound, "template not found")
	}
	if tpl.Visibility != "public" {
		return nil, status.Error(codes.NotFound, "template not found")
	}

	limit := clampInt(int(req.Limit), authorDefaultLimit, 1, 4)

	items, err := s.TemplateRepo.ListMoreFromAuthor(ctx, tpl.ID, tpl.OwnerID, userID, limit)
	if err != nil {
		zap.S().Errorf("ListMoreFromAuthor: error listing author templates for %s: %v", tpl.ID, err)
		return nil, status.Error(codes.Internal, "failed to list more from author")
	}

	resp := &pb.ListMoreFromAuthorResponse{
		Items: make([]*pb.Template, 0, len(items)),
	}
	for _, it := range items {
		pbT := s.templateModelToProto(it)
		clearModerationReasonForNonOwner(pbT, userID)
		resp.Items = append(resp.Items, pbT)
	}
	return resp, nil
}

// deriveBasis determines the explainable basis label from the top-scored
// related item: "category" when it shares the template's category, "tag" when
// it shares at least one tag, else "generic". An empty items slice yields
// "generic".
func deriveBasis(tpl *models.Template, items []*models.Template) string {
	if len(items) == 0 {
		return "generic"
	}
	top := items[0]
	if tpl.Category.String != "" && top.Category.String == tpl.Category.String {
		return "category"
	}
	if len(tpl.Tags) > 0 && len(top.Tags) > 0 {
		for _, tg := range top.Tags {
			for _, cur := range tpl.Tags {
				if tg == cur {
					return "tag"
				}
			}
		}
	}
	return "generic"
}
