package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"
	"open-prompts/backend/internal/repository"
)

const (
	collectionNameMaxLen      = 80
	collectionDescMaxLen      = 500
	collectionLimitPerUser    = 50
	collectionMemberLimit     = 100
	collectionDefaultPageSize = 20
	collectionMaxPageSize     = 50
)

// CollectionService implements the user-curated template collection RPCs.
type CollectionService struct {
	pb.UnimplementedCollectionServiceServer

	collectionRepo repository.CollectionRepository
	templateRepo   repository.TemplateRepository
	userRepo       repository.UserRepository
}

// NewCollectionService creates a new CollectionService.
func NewCollectionService(
	collectionRepo repository.CollectionRepository,
	templateRepo repository.TemplateRepository,
	userRepo repository.UserRepository,
) *CollectionService {
	return &CollectionService{
		collectionRepo: collectionRepo,
		templateRepo:   templateRepo,
		userRepo:       userRepo,
	}
}

// CreateCollection creates a collection for the caller (default private).
func (s *CollectionService) CreateCollection(ctx context.Context, req *pb.CreateCollectionRequest) (*pb.Collection, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil || user == nil {
		return nil, status.Error(codes.Unauthenticated, "user not found")
	}
	if user.Status == "banned" {
		return nil, status.Error(codes.PermissionDenied, "banned users cannot create collections")
	}

	name := strings.TrimSpace(req.Name)
	if name == "" || len([]rune(name)) > collectionNameMaxLen {
		return nil, status.Error(codes.InvalidArgument, "name must be 1-80 characters")
	}
	if len([]rune(req.Description)) > collectionDescMaxLen {
		return nil, status.Error(codes.InvalidArgument, "description must be at most 500 characters")
	}
	visibility := req.Visibility
	if visibility == "" {
		visibility = "private"
	}
	if visibility != "private" && visibility != "public" {
		return nil, status.Error(codes.InvalidArgument, "invalid visibility")
	}

	count, err := s.collectionRepo.CountActiveByOwner(ctx, userID)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to count collections")
	}
	if count >= collectionLimitPerUser {
		return nil, status.Errorf(codes.ResourceExhausted,
			"collection limit reached (%d)", collectionLimitPerUser)
	}

	c := &models.Collection{
		OwnerID:     userID,
		Name:        name,
		Description: req.Description,
		Visibility:  visibility,
		Status:      "active",
	}
	if err := s.collectionRepo.Create(ctx, c); err != nil {
		return nil, status.Error(codes.Internal, "failed to create collection")
	}
	return s.toProtoCollection(c, user, nil), nil
}

// UpdateCollection edits name/description/visibility (owner only).
func (s *CollectionService) UpdateCollection(ctx context.Context, req *pb.UpdateCollectionRequest) (*pb.Collection, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	c, err := s.collectionRepo.GetByID(ctx, req.Id)
	if err != nil {
		return nil, status.Error(codes.NotFound, "collection not found")
	}
	if c.Status == "deleted" {
		return nil, status.Error(codes.NotFound, "collection not found")
	}
	if c.OwnerID != userID {
		return nil, status.Error(codes.PermissionDenied, "not the collection owner")
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil || user == nil {
		return nil, status.Error(codes.Unauthenticated, "user not found")
	}
	if user.Status == "banned" {
		return nil, status.Error(codes.PermissionDenied, "banned users cannot update collections")
	}

	// proto3 cannot distinguish unset from empty strings; treat empty name as
	// "not provided" (clearing the name is not supported, matching UpdateTemplate).
	if req.Name != "" {
		name := strings.TrimSpace(req.Name)
		if name == "" || len([]rune(name)) > collectionNameMaxLen {
			return nil, status.Error(codes.InvalidArgument, "name must be 1-80 characters")
		}
		c.Name = name
	}
	if req.Description != "" {
		if len([]rune(req.Description)) > collectionDescMaxLen {
			return nil, status.Error(codes.InvalidArgument, "description must be at most 500 characters")
		}
		c.Description = req.Description
	}
	if req.Visibility != "" {
		if req.Visibility != "private" && req.Visibility != "public" {
			return nil, status.Error(codes.InvalidArgument, "invalid visibility")
		}
		c.Visibility = req.Visibility
	}

	if err := s.collectionRepo.Update(ctx, c); err != nil {
		return nil, status.Error(codes.Internal, "failed to update collection")
	}
	members, err := s.collectionRepo.ListMembers(ctx, c.ID, true)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to list collection members")
	}
	c.TemplateCount = int32(len(members))
	return s.toProtoCollection(c, user, members), nil
}

// DeleteCollection soft-deletes a collection (owner only).
func (s *CollectionService) DeleteCollection(ctx context.Context, req *pb.DeleteCollectionRequest) (*pb.DeleteCollectionResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	c, err := s.collectionRepo.GetByID(ctx, req.Id)
	if err != nil {
		return nil, status.Error(codes.NotFound, "collection not found")
	}
	if c.Status == "deleted" {
		return nil, status.Error(codes.NotFound, "collection not found")
	}
	if c.OwnerID != userID {
		return nil, status.Error(codes.PermissionDenied, "not the collection owner")
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil || user == nil {
		return nil, status.Error(codes.Unauthenticated, "user not found")
	}
	if user.Status == "banned" {
		return nil, status.Error(codes.PermissionDenied, "banned users cannot delete collections")
	}

	if err := s.collectionRepo.SoftDelete(ctx, req.Id); err != nil {
		return nil, status.Error(codes.Internal, "failed to delete collection")
	}
	return &pb.DeleteCollectionResponse{Id: req.Id}, nil
}

// GetCollection returns a collection with its members. Public collections are
// readable anonymously; private/deleted collections return NotFound for
// non-owners (indistinguishable from nonexistent).
func (s *CollectionService) GetCollection(ctx context.Context, req *pb.GetCollectionRequest) (*pb.Collection, error) {
	c, err := s.collectionRepo.GetByID(ctx, req.Id)
	if err != nil {
		return nil, status.Error(codes.NotFound, "collection not found")
	}
	if c.Status == "deleted" {
		return nil, status.Error(codes.NotFound, "collection not found")
	}

	userID, _ := GetUserIDFromContext(ctx) // public RPC: may be absent
	isOwner := userID != "" && c.OwnerID == userID
	if c.Visibility == "private" && !isOwner {
		return nil, status.Error(codes.NotFound, "collection not found")
	}

	owner, err := s.userRepo.GetByID(ctx, c.OwnerID)
	if err != nil || owner == nil {
		return nil, status.Error(codes.Internal, "failed to load collection owner")
	}

	members, err := s.collectionRepo.ListMembers(ctx, c.ID, isOwner)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to list collection members")
	}
	c.TemplateCount = int32(len(members))
	return s.toProtoCollection(c, owner, members), nil
}

// ListMyCollections returns the caller's active collections.
func (s *CollectionService) ListMyCollections(ctx context.Context, req *pb.ListMyCollectionsRequest) (*pb.ListMyCollectionsResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	cols, err := s.collectionRepo.ListByOwner(ctx, userID)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to list collections")
	}
	// Single owner lookup, cached per call.
	owner, _ := s.userRepo.GetByID(ctx, userID)
	resp := &pb.ListMyCollectionsResponse{Collections: make([]*pb.Collection, 0, len(cols))}
	for _, c := range cols {
		resp.Collections = append(resp.Collections, s.toProtoCollection(c, owner, nil))
	}
	return resp, nil
}

// ListPublicCollections returns the public explore feed, paginated and sorted.
func (s *CollectionService) ListPublicCollections(ctx context.Context, req *pb.ListPublicCollectionsRequest) (*pb.ListPublicCollectionsResponse, error) {
	limit := int(req.PageSize)
	if req.PageSize == 0 {
		limit = collectionDefaultPageSize
	}
	if limit < 1 || limit > collectionMaxPageSize {
		return nil, status.Error(codes.InvalidArgument, "page_size must be between 1 and 50")
	}

	sort := req.Sort
	if sort == "" {
		sort = "newest"
	}
	if sort != "newest" && sort != "most_templates" {
		return nil, status.Error(codes.InvalidArgument, "invalid sort")
	}

	offset := 0
	if req.PageToken != "" {
		var perr error
		offset, perr = strconv.Atoi(req.PageToken)
		if perr != nil || offset < 0 {
			return nil, status.Error(codes.InvalidArgument, "invalid page_token")
		}
	}

	cols, err := s.collectionRepo.ListPublic(ctx, int32(offset), int32(limit), sort)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to list public collections")
	}

	// Owner lookups, cached per call.
	ownerCache := map[string]*models.User{}
	resp := &pb.ListPublicCollectionsResponse{Collections: make([]*pb.Collection, 0, len(cols))}
	for _, c := range cols {
		owner := ownerCache[c.OwnerID]
		if owner == nil {
			owner, _ = s.userRepo.GetByID(ctx, c.OwnerID)
			ownerCache[c.OwnerID] = owner
		}
		resp.Collections = append(resp.Collections, s.toProtoCollection(c, owner, nil))
	}
	if len(cols) == limit {
		resp.NextPageToken = strconv.Itoa(offset + limit)
	}
	return resp, nil
}

// AddTemplateToCollection adds a template (owner only, idempotent).
func (s *CollectionService) AddTemplateToCollection(ctx context.Context, req *pb.AddTemplateToCollectionRequest) (*pb.AddTemplateToCollectionResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	c, err := s.collectionRepo.GetByID(ctx, req.CollectionId)
	if err != nil {
		return nil, status.Error(codes.NotFound, "collection not found")
	}
	if c.Status == "deleted" {
		return nil, status.Error(codes.NotFound, "collection not found")
	}
	if c.OwnerID != userID {
		return nil, status.Error(codes.PermissionDenied, "not the collection owner")
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil || user == nil {
		return nil, status.Error(codes.Unauthenticated, "user not found")
	}
	if user.Status == "banned" {
		return nil, status.Error(codes.PermissionDenied, "banned users cannot modify collections")
	}

	t, err := s.templateRepo.Get(ctx, req.TemplateId, userID)
	if err != nil {
		return nil, status.Error(codes.NotFound, "template not found")
	}
	if t.Visibility == "private" && t.OwnerID != userID {
		return nil, status.Error(codes.NotFound, "template not found") // indistinguishable
	}

	count, err := s.collectionRepo.CountMembers(ctx, req.CollectionId)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to count collection members")
	}
	if count >= collectionMemberLimit {
		return nil, status.Errorf(codes.ResourceExhausted,
			"collection member limit reached (%d)", collectionMemberLimit)
	}

	// Idempotency: if already present, return the current position and count.
	existingPos, err := s.collectionRepo.GetMemberPosition(ctx, req.CollectionId, req.TemplateId)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to read collection position")
	}
	if existingPos >= 0 {
		newCount, err := s.collectionRepo.CountMembers(ctx, req.CollectionId)
		if err != nil {
			return nil, status.Error(codes.Internal, "failed to count collection members")
		}
		return &pb.AddTemplateToCollectionResponse{
			CollectionId:  req.CollectionId,
			TemplateId:    req.TemplateId,
			Position:      existingPos,
			TemplateCount: newCount,
		}, nil
	}

	pos, err := s.collectionRepo.MaxMemberPosition(ctx, req.CollectionId)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to read collection position")
	}
	nextPos := pos + 1
	if err := s.collectionRepo.AddMember(ctx, req.CollectionId, req.TemplateId, nextPos); err != nil {
		return nil, status.Error(codes.Internal, "failed to add template to collection")
	}

	// Re-read the count so the response reports the true post-add count.
	newCount, err := s.collectionRepo.CountMembers(ctx, req.CollectionId)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to count collection members")
	}
	return &pb.AddTemplateToCollectionResponse{
		CollectionId:  req.CollectionId,
		TemplateId:    req.TemplateId,
		Position:      nextPos,
		TemplateCount: newCount,
	}, nil
}

// RemoveTemplateFromCollection removes a template (owner only).
func (s *CollectionService) RemoveTemplateFromCollection(ctx context.Context, req *pb.RemoveTemplateFromCollectionRequest) (*emptypb.Empty, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	c, err := s.collectionRepo.GetByID(ctx, req.CollectionId)
	if err != nil {
		return nil, status.Error(codes.NotFound, "collection not found")
	}
	if c.Status == "deleted" {
		return nil, status.Error(codes.NotFound, "collection not found")
	}
	if c.OwnerID != userID {
		return nil, status.Error(codes.PermissionDenied, "not the collection owner")
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil || user == nil {
		return nil, status.Error(codes.Unauthenticated, "user not found")
	}
	if user.Status == "banned" {
		return nil, status.Error(codes.PermissionDenied, "banned users cannot modify collections")
	}

	if err := s.collectionRepo.RemoveMember(ctx, req.CollectionId, req.TemplateId); err != nil {
		return nil, status.Error(codes.Internal, "failed to remove template from collection")
	}
	return &emptypb.Empty{}, nil
}

// ReorderCollectionTemplates applies a full ordered template-id list (owner only).
func (s *CollectionService) ReorderCollectionTemplates(ctx context.Context, req *pb.ReorderCollectionTemplatesRequest) (*emptypb.Empty, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	c, err := s.collectionRepo.GetByID(ctx, req.CollectionId)
	if err != nil {
		return nil, status.Error(codes.NotFound, "collection not found")
	}
	if c.Status == "deleted" {
		return nil, status.Error(codes.NotFound, "collection not found")
	}
	if c.OwnerID != userID {
		return nil, status.Error(codes.PermissionDenied, "not the collection owner")
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil || user == nil {
		return nil, status.Error(codes.Unauthenticated, "user not found")
	}
	if user.Status == "banned" {
		return nil, status.Error(codes.PermissionDenied, "banned users cannot modify collections")
	}

	if len(req.TemplateIds) > collectionMemberLimit {
		return nil, status.Error(codes.InvalidArgument, "too many templates")
	}
	if hasDuplicates(req.TemplateIds) {
		return nil, status.Error(codes.InvalidArgument, "duplicate template ids")
	}

	// Every id must be a current member (NotFound otherwise) — the client
	// sends the full list, so a mismatch means stale state.
	current, err := s.collectionRepo.ListMembers(ctx, req.CollectionId, true)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to list collection members")
	}
	currentSet := make(map[string]struct{}, len(current))
	for _, m := range current {
		currentSet[m.Template.ID] = struct{}{}
	}
	reqSet := make(map[string]struct{}, len(req.TemplateIds))
	for _, id := range req.TemplateIds {
		reqSet[id] = struct{}{}
	}
	if len(currentSet) != len(reqSet) {
		return nil, status.Error(codes.NotFound, "collection members changed")
	}
	for id := range reqSet {
		if _, ok := currentSet[id]; !ok {
			return nil, status.Error(codes.NotFound, "collection members changed")
		}
	}

	if err := s.collectionRepo.ReorderMembers(ctx, req.CollectionId, req.TemplateIds); err != nil {
		return nil, status.Error(codes.Internal, "failed to reorder collection")
	}
	return &emptypb.Empty{}, nil
}

// ListCollectionsContainingTemplate returns the caller's collections that
// contain a given template (used for picker checkmarks).
func (s *CollectionService) ListCollectionsContainingTemplate(ctx context.Context, req *pb.ListCollectionsContainingTemplateRequest) (*pb.ListCollectionsContainingTemplateResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	t, err := s.templateRepo.Get(ctx, req.TemplateId, userID)
	if err != nil {
		return nil, status.Error(codes.NotFound, "template not found")
	}
	if t.Visibility == "private" && t.OwnerID != userID {
		return nil, status.Error(codes.NotFound, "template not found") // indistinguishable
	}

	cols, err := s.collectionRepo.ListContainingTemplate(ctx, userID, req.TemplateId)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to list collections containing template")
	}
	owner, _ := s.userRepo.GetByID(ctx, userID)
	resp := &pb.ListCollectionsContainingTemplateResponse{Collections: make([]*pb.Collection, 0, len(cols))}
	for _, c := range cols {
		resp.Collections = append(resp.Collections, s.toProtoCollection(c, owner, nil))
	}
	return resp, nil
}

// toProtoCollection maps a model collection to its proto form. members is
// nil for list contexts (GetCollection fills it).
func (s *CollectionService) toProtoCollection(c *models.Collection, owner *models.User, members []*models.CollectionMember) *pb.Collection {
	out := &pb.Collection{
		Id:            c.ID,
		OwnerId:       c.OwnerID,
		Name:          c.Name,
		Description:   c.Description,
		Visibility:    c.Visibility,
		TemplateCount: c.TemplateCount,
		CreatedAt:     c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     c.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if owner != nil {
		out.Owner = &pb.CollectionOwner{Id: owner.ID, DisplayName: owner.DisplayName}
	}
	for _, m := range members {
		out.Members = append(out.Members, &pb.CollectionMember{
			Template: s.collectionTemplateToProto(&m.Template),
			Position: m.Position,
			IsPublic: m.IsPublic,
		})
	}
	return out
}

// collectionTemplateToProto maps a template model to its proto form, matching
// the payload shape of GET /api/v1/templates/{id}.
func (s *CollectionService) collectionTemplateToProto(m *models.Template) *pb.Template {
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
	}
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
	return tpl
}

// hasDuplicates reports whether a slice contains any duplicate value.
func hasDuplicates(ids []string) bool {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			return true
		}
		seen[id] = struct{}{}
	}
	return false
}
