package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MockCollectionRepository is a mock of repository.CollectionRepository.
type MockCollectionRepository struct {
	mock.Mock
}

func (m *MockCollectionRepository) Create(ctx context.Context, c *models.Collection) error {
	args := m.Called(ctx, c)
	return args.Error(0)
}
func (m *MockCollectionRepository) Update(ctx context.Context, c *models.Collection) error {
	args := m.Called(ctx, c)
	return args.Error(0)
}
func (m *MockCollectionRepository) SoftDelete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
func (m *MockCollectionRepository) GetByID(ctx context.Context, id string) (*models.Collection, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Collection), args.Error(1)
}
func (m *MockCollectionRepository) CountActiveByOwner(ctx context.Context, ownerID string) (int32, error) {
	args := m.Called(ctx, ownerID)
	return args.Get(0).(int32), args.Error(1)
}
func (m *MockCollectionRepository) ListByOwner(ctx context.Context, ownerID string) ([]*models.Collection, error) {
	args := m.Called(ctx, ownerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Collection), args.Error(1)
}
func (m *MockCollectionRepository) ListPublic(ctx context.Context, offset, limit int32, sort string) ([]*models.Collection, error) {
	args := m.Called(ctx, offset, limit, sort)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Collection), args.Error(1)
}
func (m *MockCollectionRepository) CountPublic(ctx context.Context) (int32, error) {
	args := m.Called(ctx)
	return args.Get(0).(int32), args.Error(1)
}
func (m *MockCollectionRepository) AddMember(ctx context.Context, collectionID, templateID string, position int32) error {
	args := m.Called(ctx, collectionID, templateID, position)
	return args.Error(0)
}
func (m *MockCollectionRepository) RemoveMember(ctx context.Context, collectionID, templateID string) error {
	args := m.Called(ctx, collectionID, templateID)
	return args.Error(0)
}
func (m *MockCollectionRepository) ReorderMembers(ctx context.Context, collectionID string, templateIDs []string) error {
	args := m.Called(ctx, collectionID, templateIDs)
	return args.Error(0)
}
func (m *MockCollectionRepository) CountMembers(ctx context.Context, collectionID string) (int32, error) {
	args := m.Called(ctx, collectionID)
	return args.Get(0).(int32), args.Error(1)
}
func (m *MockCollectionRepository) MaxMemberPosition(ctx context.Context, collectionID string) (int32, error) {
	args := m.Called(ctx, collectionID)
	return args.Get(0).(int32), args.Error(1)
}
func (m *MockCollectionRepository) GetMemberPosition(ctx context.Context, collectionID, templateID string) (int32, error) {
	args := m.Called(ctx, collectionID, templateID)
	return args.Get(0).(int32), args.Error(1)
}
func (m *MockCollectionRepository) ListMembers(ctx context.Context, collectionID string, ownerView bool) ([]*models.CollectionMember, error) {
	args := m.Called(ctx, collectionID, ownerView)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.CollectionMember), args.Error(1)
}
func (m *MockCollectionRepository) ListContainingTemplate(ctx context.Context, ownerID, templateID string) ([]*models.Collection, error) {
	args := m.Called(ctx, ownerID, templateID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Collection), args.Error(1)
}

func newCollectionTestService(t *testing.T) (*CollectionService, *MockCollectionRepository, *MockTemplateRepository, *MockUserRepository) {
	t.Helper()
	collectionRepo := new(MockCollectionRepository)
	templateRepo := new(MockTemplateRepository)
	userRepo := new(MockUserRepository)
	svc := NewCollectionService(collectionRepo, templateRepo, userRepo)
	return svc, collectionRepo, templateRepo, userRepo
}

func activeUser(id string) *models.User {
	return &models.User{ID: id, DisplayName: "Ada", Status: "active"}
}

func bannedUser(id string) *models.User {
	return &models.User{ID: id, DisplayName: "Bad", Status: "banned"}
}

func activeCollection(id, ownerID string) *models.Collection {
	return &models.Collection{
		ID: id, OwnerID: ownerID, Name: "My collection",
		Description: "desc", Visibility: "private", Status: "active",
	}
}

func TestCreateCollection_Validation(t *testing.T) {
	svc, _, _, userRepo := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "user-1")
	userRepo.On("GetByID", mock.Anything, "user-1").Return(activeUser("user-1"), nil)
	userRepo.On("CountActiveByOwner", mock.Anything, "user-1").Return(int32(0), nil)

	// Missing name -> 400.
	_, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{Name: "  "})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	// Name > 80 -> 400.
	longName := make([]rune, 81)
	for i := range longName {
		longName[i] = 'a'
	}
	_, err = svc.CreateCollection(ctx, &pb.CreateCollectionRequest{Name: string(longName)})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	// Description > 500 -> 400.
	longDesc := make([]rune, 501)
	for i := range longDesc {
		longDesc[i] = 'b'
	}
	_, err = svc.CreateCollection(ctx, &pb.CreateCollectionRequest{Name: "ok", Description: string(longDesc)})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	// Invalid visibility -> 400.
	_, err = svc.CreateCollection(ctx, &pb.CreateCollectionRequest{Name: "ok", Visibility: "secret"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestCreateCollection_UnauthenticatedAndBanned(t *testing.T) {
	svc, _, _, _ := newCollectionTestService(t)

	// No user in context -> 401.
	_, err := svc.CreateCollection(context.Background(), &pb.CreateCollectionRequest{Name: "ok"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	// Banned user -> 403.
	svc2, _, _, userRepo := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "banned-1")
	userRepo.On("GetByID", mock.Anything, "banned-1").Return(bannedUser("banned-1"), nil)
	_, err = svc2.CreateCollection(ctx, &pb.CreateCollectionRequest{Name: "ok"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestCreateCollection_LimitReached(t *testing.T) {
	svc, collectionRepo, _, userRepo := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "user-1")
	userRepo.On("GetByID", mock.Anything, "user-1").Return(activeUser("user-1"), nil)
	collectionRepo.On("CountActiveByOwner", mock.Anything, "user-1").Return(int32(50), nil)

	_, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{Name: "ok"})
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
}

func TestCreateCollection_Success(t *testing.T) {
	svc, collectionRepo, _, userRepo := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "user-1")
	userRepo.On("GetByID", mock.Anything, "user-1").Return(activeUser("user-1"), nil)
	collectionRepo.On("CountActiveByOwner", mock.Anything, "user-1").Return(int32(0), nil)
	collectionRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Collection")).
		Return(nil).Run(func(args mock.Arguments) {
		c := args.Get(1).(*models.Collection)
		c.ID = "col-1"
	})

	resp, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{Name: "My collection", Description: "desc"})
	require.NoError(t, err)
	assert.Equal(t, "col-1", resp.Id)
	assert.Equal(t, "private", resp.Visibility) // default private
	assert.Equal(t, "user-1", resp.OwnerId)
	assert.Equal(t, "Ada", resp.Owner.DisplayName)
}

func TestGetCollection_Visibility(t *testing.T) {
	svc, collectionRepo, _, userRepo := newCollectionTestService(t)

	// Nonexistent -> 404.
	collectionRepo.On("GetByID", mock.Anything, "missing").Return(nil, sql.ErrNoRows)
	_, err := svc.GetCollection(context.Background(), &pb.GetCollectionRequest{Id: "missing"})
	assert.Equal(t, codes.NotFound, status.Code(err))

	// Deleted -> 404 for everyone.
	deleted := activeCollection("col-1", "owner-1")
	deleted.Status = "deleted"
	collectionRepo.On("GetByID", mock.Anything, "col-1").Return(deleted, nil)
	_, err = svc.GetCollection(context.Background(), &pb.GetCollectionRequest{Id: "col-1"})
	assert.Equal(t, codes.NotFound, status.Code(err))

	// Private collection, non-owner (anonymous) -> 404.
	collectionRepo.On("GetByID", mock.Anything, "col-2").Return(activeCollection("col-2", "owner-1"), nil)
	_, err = svc.GetCollection(context.Background(), &pb.GetCollectionRequest{Id: "col-2"})
	assert.Equal(t, codes.NotFound, status.Code(err))

	// Private collection, other user -> 404.
	ctx := ContextWithUserID(context.Background(), "other")
	_, err = svc.GetCollection(ctx, &pb.GetCollectionRequest{Id: "col-2"})
	assert.Equal(t, codes.NotFound, status.Code(err))

	// Private collection, owner -> 200 with members.
	ownerCtx := ContextWithUserID(context.Background(), "owner-1")
	userRepo.On("GetByID", mock.Anything, "owner-1").Return(activeUser("owner-1"), nil)
	collectionRepo.On("ListMembers", mock.Anything, "col-2", true).Return([]*models.CollectionMember{
		{Template: models.Template{ID: "t-1", Visibility: "public"}, Position: 0, IsPublic: true},
	}, nil)
	resp, err := svc.GetCollection(ownerCtx, &pb.GetCollectionRequest{Id: "col-2"})
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.TemplateCount)
	assert.Len(t, resp.Members, 1)
}

func TestGetCollection_PublicAnonymous(t *testing.T) {
	svc, collectionRepo, _, userRepo := newCollectionTestService(t)
	pub := activeCollection("col-pub", "owner-1")
	pub.Visibility = "public"
	collectionRepo.On("GetByID", mock.Anything, "col-pub").Return(pub, nil)
	userRepo.On("GetByID", mock.Anything, "owner-1").Return(activeUser("owner-1"), nil)
	// Public view filters to public templates only.
	collectionRepo.On("ListMembers", mock.Anything, "col-pub", false).Return([]*models.CollectionMember{
		{Template: models.Template{ID: "t-1", Visibility: "public"}, Position: 0, IsPublic: true},
	}, nil)

	resp, err := svc.GetCollection(context.Background(), &pb.GetCollectionRequest{Id: "col-pub"})
	require.NoError(t, err)
	assert.Equal(t, "public", resp.Visibility)
	assert.Equal(t, int32(1), resp.TemplateCount)
	assert.Equal(t, "Ada", resp.Owner.DisplayName)
}

func TestUpdateCollection_OwnerOnly(t *testing.T) {
	svc, collectionRepo, _, userRepo := newCollectionTestService(t)

	// Non-owner -> 403.
	collectionRepo.On("GetByID", mock.Anything, "col-1").Return(activeCollection("col-1", "owner-1"), nil)
	ctx := ContextWithUserID(context.Background(), "other")
	_, err := svc.UpdateCollection(ctx, &pb.UpdateCollectionRequest{Id: "col-1", Name: "new"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	// Owner success.
	ownerCtx := ContextWithUserID(context.Background(), "owner-1")
	userRepo.On("GetByID", mock.Anything, "owner-1").Return(activeUser("owner-1"), nil)
	collectionRepo.On("Update", mock.Anything, mock.AnythingOfType("*models.Collection")).Return(nil)
	collectionRepo.On("ListMembers", mock.Anything, "col-1", true).Return([]*models.CollectionMember{}, nil)
	resp, err := svc.UpdateCollection(ownerCtx, &pb.UpdateCollectionRequest{Id: "col-1", Name: "new name", Visibility: "public"})
	require.NoError(t, err)
	assert.Equal(t, "new name", resp.Name)
	assert.Equal(t, "public", resp.Visibility)
}

func TestUpdateCollection_Validation(t *testing.T) {
	svc, collectionRepo, _, userRepo := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "owner-1")
	collectionRepo.On("GetByID", mock.Anything, "col-1").Return(activeCollection("col-1", "owner-1"), nil)
	userRepo.On("GetByID", mock.Anything, "owner-1").Return(activeUser("owner-1"), nil)

	// Name > 80 -> 400.
	longName := make([]rune, 81)
	for i := range longName {
		longName[i] = 'a'
	}
	_, err := svc.UpdateCollection(ctx, &pb.UpdateCollectionRequest{Id: "col-1", Name: string(longName)})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	// Description > 500 -> 400.
	longDesc := make([]rune, 501)
	for i := range longDesc {
		longDesc[i] = 'b'
	}
	_, err = svc.UpdateCollection(ctx, &pb.UpdateCollectionRequest{Id: "col-1", Description: string(longDesc)})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	// Invalid visibility -> 400.
	_, err = svc.UpdateCollection(ctx, &pb.UpdateCollectionRequest{Id: "col-1", Visibility: "secret"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestRemoveTemplateFromCollection_NonOwner(t *testing.T) {
	svc, collectionRepo, _, _ := newCollectionTestService(t)
	collectionRepo.On("GetByID", mock.Anything, "col-1").Return(activeCollection("col-1", "owner-1"), nil)
	ctx := ContextWithUserID(context.Background(), "other")
	_, err := svc.RemoveTemplateFromCollection(ctx, &pb.RemoveTemplateFromCollectionRequest{CollectionId: "col-1", TemplateId: "t-1"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestDeleteCollection_OwnerOnly(t *testing.T) {
	svc, collectionRepo, _, userRepo := newCollectionTestService(t)

	// Non-owner -> 403.
	collectionRepo.On("GetByID", mock.Anything, "col-1").Return(activeCollection("col-1", "owner-1"), nil)
	ctx := ContextWithUserID(context.Background(), "other")
	_, err := svc.DeleteCollection(ctx, &pb.DeleteCollectionRequest{Id: "col-1"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	// Owner success.
	ownerCtx := ContextWithUserID(context.Background(), "owner-1")
	userRepo.On("GetByID", mock.Anything, "owner-1").Return(activeUser("owner-1"), nil)
	collectionRepo.On("SoftDelete", mock.Anything, "col-1").Return(nil)
	resp, err := svc.DeleteCollection(ownerCtx, &pb.DeleteCollectionRequest{Id: "col-1"})
	require.NoError(t, err)
	assert.Equal(t, "col-1", resp.Id)
}

func TestListPublicCollections_Validation(t *testing.T) {
	svc, _, _, _ := newCollectionTestService(t)

	// Invalid sort -> 400.
	_, err := svc.ListPublicCollections(context.Background(), &pb.ListPublicCollectionsRequest{Sort: "bogus"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	// page_size > 50 -> 400.
	_, err = svc.ListPublicCollections(context.Background(), &pb.ListPublicCollectionsRequest{PageSize: 100})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	// Invalid page_token -> 400.
	_, err = svc.ListPublicCollections(context.Background(), &pb.ListPublicCollectionsRequest{PageToken: "abc"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestListPublicCollections_SuccessAndPagination(t *testing.T) {
	svc, collectionRepo, _, userRepo := newCollectionTestService(t)
	collectionRepo.On("ListPublic", mock.Anything, int32(0), int32(20), "newest").
		Return([]*models.Collection{activeCollection("col-1", "owner-1")}, nil)
	userRepo.On("GetByID", mock.Anything, "owner-1").Return(activeUser("owner-1"), nil)

	resp, err := svc.ListPublicCollections(context.Background(), &pb.ListPublicCollectionsRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.Collections, 1)
	assert.Equal(t, "", resp.NextPageToken) // fewer than page_size -> no next

	// Full page -> next_page_token set.
	collectionRepo.On("ListPublic", mock.Anything, int32(0), int32(2), "newest").
		Return([]*models.Collection{
			activeCollection("col-1", "owner-1"),
			activeCollection("col-2", "owner-1"),
		}, nil)
	resp, err = svc.ListPublicCollections(context.Background(), &pb.ListPublicCollectionsRequest{PageSize: 2})
	require.NoError(t, err)
	assert.Equal(t, "2", resp.NextPageToken)
}

func TestListMyCollections(t *testing.T) {
	svc, collectionRepo, _, userRepo := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "user-1")
	collectionRepo.On("ListByOwner", mock.Anything, "user-1").Return([]*models.Collection{
		activeCollection("col-1", "user-1"),
	}, nil)
	userRepo.On("GetByID", mock.Anything, "user-1").Return(activeUser("user-1"), nil)

	resp, err := svc.ListMyCollections(ctx, &pb.ListMyCollectionsRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.Collections, 1)
	assert.Equal(t, "col-1", resp.Collections[0].Id)
}

func TestAddTemplateToCollection(t *testing.T) {
	svc, collectionRepo, templateRepo, userRepo := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "owner-1")

	// Non-owner -> 403.
	collectionRepo.On("GetByID", mock.Anything, "col-1").Return(activeCollection("col-1", "owner-1"), nil)
	otherCtx := ContextWithUserID(context.Background(), "other")
	_, err := svc.AddTemplateToCollection(otherCtx, &pb.AddTemplateToCollectionRequest{CollectionId: "col-1", TemplateId: "t-1"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	// Owner adds a public template.
	userRepo.On("GetByID", mock.Anything, "owner-1").Return(activeUser("owner-1"), nil)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return &models.Template{ID: "t-1", OwnerID: "someone", Visibility: "public"}, nil
	}
	collectionRepo.On("CountMembers", mock.Anything, "col-1").Return(int32(0), nil).Once()
	collectionRepo.On("GetMemberPosition", mock.Anything, "col-1", "t-1").Return(int32(-1), nil)
	collectionRepo.On("MaxMemberPosition", mock.Anything, "col-1").Return(int32(-1), nil)
	collectionRepo.On("AddMember", mock.Anything, "col-1", "t-1", int32(0)).Return(nil)
	collectionRepo.On("CountMembers", mock.Anything, "col-1").Return(int32(1), nil).Once()

	resp, err := svc.AddTemplateToCollection(ctx, &pb.AddTemplateToCollectionRequest{CollectionId: "col-1", TemplateId: "t-1"})
	require.NoError(t, err)
	assert.Equal(t, int32(0), resp.Position)
	assert.Equal(t, int32(1), resp.TemplateCount)
}

func TestAddTemplateToCollection_Idempotent(t *testing.T) {
	svc, collectionRepo, templateRepo, userRepo := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "owner-1")
	collectionRepo.On("GetByID", mock.Anything, "col-1").Return(activeCollection("col-1", "owner-1"), nil)
	userRepo.On("GetByID", mock.Anything, "owner-1").Return(activeUser("owner-1"), nil)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return &models.Template{ID: "t-1", OwnerID: "someone", Visibility: "public"}, nil
	}
	collectionRepo.On("CountMembers", mock.Anything, "col-1").Return(int32(1), nil).Once()
	// Already present at position 0 -> return existing position + count.
	collectionRepo.On("GetMemberPosition", mock.Anything, "col-1", "t-1").Return(int32(0), nil)
	collectionRepo.On("CountMembers", mock.Anything, "col-1").Return(int32(1), nil).Once()

	resp, err := svc.AddTemplateToCollection(ctx, &pb.AddTemplateToCollectionRequest{CollectionId: "col-1", TemplateId: "t-1"})
	require.NoError(t, err)
	assert.Equal(t, int32(0), resp.Position)
	assert.Equal(t, int32(1), resp.TemplateCount)
}

func TestAddTemplateToCollection_PrivateTemplateNonOwner(t *testing.T) {
	svc, collectionRepo, templateRepo, userRepo := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "owner-1")
	collectionRepo.On("GetByID", mock.Anything, "col-1").Return(activeCollection("col-1", "owner-1"), nil)
	userRepo.On("GetByID", mock.Anything, "owner-1").Return(activeUser("owner-1"), nil)
	// Private template owned by someone else -> 404 (no existence leak).
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return &models.Template{ID: "t-1", OwnerID: "someone", Visibility: "private"}, nil
	}
	_, err := svc.AddTemplateToCollection(ctx, &pb.AddTemplateToCollectionRequest{CollectionId: "col-1", TemplateId: "t-1"})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestAddTemplateToCollection_MemberLimit(t *testing.T) {
	svc, collectionRepo, templateRepo, userRepo := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "owner-1")
	collectionRepo.On("GetByID", mock.Anything, "col-1").Return(activeCollection("col-1", "owner-1"), nil)
	userRepo.On("GetByID", mock.Anything, "owner-1").Return(activeUser("owner-1"), nil)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return &models.Template{ID: "t-1", OwnerID: "someone", Visibility: "public"}, nil
	}
	collectionRepo.On("CountMembers", mock.Anything, "col-1").Return(int32(100), nil)

	_, err := svc.AddTemplateToCollection(ctx, &pb.AddTemplateToCollectionRequest{CollectionId: "col-1", TemplateId: "t-1"})
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
}

func TestRemoveTemplateFromCollection_OwnerOnly(t *testing.T) {
	svc, collectionRepo, _, userRepo := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "owner-1")
	collectionRepo.On("GetByID", mock.Anything, "col-1").Return(activeCollection("col-1", "owner-1"), nil)
	userRepo.On("GetByID", mock.Anything, "owner-1").Return(activeUser("owner-1"), nil)
	collectionRepo.On("RemoveMember", mock.Anything, "col-1", "t-1").Return(nil)

	_, err := svc.RemoveTemplateFromCollection(ctx, &pb.RemoveTemplateFromCollectionRequest{CollectionId: "col-1", TemplateId: "t-1"})
	require.NoError(t, err)
}

func TestReorderCollectionTemplates(t *testing.T) {
	svc, collectionRepo, _, userRepo := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "owner-1")
	collectionRepo.On("GetByID", mock.Anything, "col-1").Return(activeCollection("col-1", "owner-1"), nil)
	userRepo.On("GetByID", mock.Anything, "owner-1").Return(activeUser("owner-1"), nil)

	// Duplicate ids -> 400.
	_, err := svc.ReorderCollectionTemplates(ctx, &pb.ReorderCollectionTemplatesRequest{
		CollectionId: "col-1", TemplateIds: []string{"t-1", "t-1"},
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	// Mismatch with current members -> 404.
	collectionRepo.On("ListMembers", mock.Anything, "col-1", true).Return([]*models.CollectionMember{
		{Template: models.Template{ID: "t-1"}, Position: 0},
		{Template: models.Template{ID: "t-2"}, Position: 1},
	}, nil)
	_, err = svc.ReorderCollectionTemplates(ctx, &pb.ReorderCollectionTemplatesRequest{
		CollectionId: "col-1", TemplateIds: []string{"t-1", "t-3"},
	})
	assert.Equal(t, codes.NotFound, status.Code(err))

	// Success.
	collectionRepo.On("ReorderMembers", mock.Anything, "col-1", []string{"t-2", "t-1"}).Return(nil)
	_, err = svc.ReorderCollectionTemplates(ctx, &pb.ReorderCollectionTemplatesRequest{
		CollectionId: "col-1", TemplateIds: []string{"t-2", "t-1"},
	})
	require.NoError(t, err)
}

func TestListCollectionsContainingTemplate(t *testing.T) {
	svc, collectionRepo, templateRepo, userRepo := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "user-1")
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return &models.Template{ID: "t-1", OwnerID: "someone", Visibility: "public"}, nil
	}
	collectionRepo.On("ListContainingTemplate", mock.Anything, "user-1", "t-1").Return([]*models.Collection{
		activeCollection("col-1", "user-1"),
	}, nil)
	userRepo.On("GetByID", mock.Anything, "user-1").Return(activeUser("user-1"), nil)

	resp, err := svc.ListCollectionsContainingTemplate(ctx, &pb.ListCollectionsContainingTemplateRequest{TemplateId: "t-1"})
	require.NoError(t, err)
	assert.Len(t, resp.Collections, 1)
	assert.Equal(t, "col-1", resp.Collections[0].Id)
}

func TestListCollectionsContainingTemplate_PrivateNonOwner(t *testing.T) {
	svc, _, templateRepo, _ := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "user-1")
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return &models.Template{ID: "t-1", OwnerID: "someone", Visibility: "private"}, nil
	}
	_, err := svc.ListCollectionsContainingTemplate(ctx, &pb.ListCollectionsContainingTemplateRequest{TemplateId: "t-1"})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestCollectionService_RepoErrors(t *testing.T) {
	svc, collectionRepo, _, userRepo := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "user-1")
	userRepo.On("GetByID", mock.Anything, "user-1").Return(activeUser("user-1"), nil)
	collectionRepo.On("CountActiveByOwner", mock.Anything, "user-1").Return(int32(0), nil)
	collectionRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Collection")).
		Return(errors.New("db down"))

	_, err := svc.CreateCollection(ctx, &pb.CreateCollectionRequest{Name: "ok"})
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestCollectionMutations_BannedUser(t *testing.T) {
	svc, collectionRepo, _, userRepo := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "banned-1")
	collectionRepo.On("GetByID", mock.Anything, "col-1").Return(activeCollection("col-1", "banned-1"), nil)
	userRepo.On("GetByID", mock.Anything, "banned-1").Return(bannedUser("banned-1"), nil)

	// Update -> 403.
	_, err := svc.UpdateCollection(ctx, &pb.UpdateCollectionRequest{Id: "col-1", Name: "x"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	// Delete -> 403.
	_, err = svc.DeleteCollection(ctx, &pb.DeleteCollectionRequest{Id: "col-1"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	// Add -> 403.
	_, err = svc.AddTemplateToCollection(ctx, &pb.AddTemplateToCollectionRequest{CollectionId: "col-1", TemplateId: "t-1"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	// Remove -> 403.
	_, err = svc.RemoveTemplateFromCollection(ctx, &pb.RemoveTemplateFromCollectionRequest{CollectionId: "col-1", TemplateId: "t-1"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	// Reorder -> 403.
	_, err = svc.ReorderCollectionTemplates(ctx, &pb.ReorderCollectionTemplatesRequest{CollectionId: "col-1", TemplateIds: []string{"t-1"}})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestCollectionMutations_DeletedCollection(t *testing.T) {
	svc, collectionRepo, _, _ := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "owner-1")
	deleted := activeCollection("col-1", "owner-1")
	deleted.Status = "deleted"
	collectionRepo.On("GetByID", mock.Anything, "col-1").Return(deleted, nil)

	// Update/Delete/Add/Remove/Reorder on a deleted collection -> 404.
	_, err := svc.UpdateCollection(ctx, &pb.UpdateCollectionRequest{Id: "col-1", Name: "x"})
	assert.Equal(t, codes.NotFound, status.Code(err))
	_, err = svc.DeleteCollection(ctx, &pb.DeleteCollectionRequest{Id: "col-1"})
	assert.Equal(t, codes.NotFound, status.Code(err))
	_, err = svc.AddTemplateToCollection(ctx, &pb.AddTemplateToCollectionRequest{CollectionId: "col-1", TemplateId: "t-1"})
	assert.Equal(t, codes.NotFound, status.Code(err))
	_, err = svc.RemoveTemplateFromCollection(ctx, &pb.RemoveTemplateFromCollectionRequest{CollectionId: "col-1", TemplateId: "t-1"})
	assert.Equal(t, codes.NotFound, status.Code(err))
	_, err = svc.ReorderCollectionTemplates(ctx, &pb.ReorderCollectionTemplatesRequest{CollectionId: "col-1", TemplateIds: []string{"t-1"}})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestCollectionMutations_RepoErrors(t *testing.T) {
	svc, collectionRepo, templateRepo, userRepo := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "owner-1")
	collectionRepo.On("GetByID", mock.Anything, "col-1").Return(activeCollection("col-1", "owner-1"), nil)
	userRepo.On("GetByID", mock.Anything, "owner-1").Return(activeUser("owner-1"), nil)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return &models.Template{ID: "t-1", OwnerID: "someone", Visibility: "public"}, nil
	}

	// Update repo error -> 500.
	collectionRepo.On("Update", mock.Anything, mock.AnythingOfType("*models.Collection")).Return(errors.New("db down"))
	_, err := svc.UpdateCollection(ctx, &pb.UpdateCollectionRequest{Id: "col-1", Name: "x"})
	assert.Equal(t, codes.Internal, status.Code(err))

	// Delete repo error -> 500.
	collectionRepo.On("SoftDelete", mock.Anything, "col-1").Return(errors.New("db down"))
	_, err = svc.DeleteCollection(ctx, &pb.DeleteCollectionRequest{Id: "col-1"})
	assert.Equal(t, codes.Internal, status.Code(err))

	// Add repo error (CountMembers) -> 500.
	collectionRepo.On("CountMembers", mock.Anything, "col-1").Return(int32(0), errors.New("db down"))
	_, err = svc.AddTemplateToCollection(ctx, &pb.AddTemplateToCollectionRequest{CollectionId: "col-1", TemplateId: "t-1"})
	assert.Equal(t, codes.Internal, status.Code(err))

	// Remove repo error -> 500.
	collectionRepo.On("RemoveMember", mock.Anything, "col-1", "t-1").Return(errors.New("db down"))
	_, err = svc.RemoveTemplateFromCollection(ctx, &pb.RemoveTemplateFromCollectionRequest{CollectionId: "col-1", TemplateId: "t-1"})
	assert.Equal(t, codes.Internal, status.Code(err))

	// Reorder repo error (ListMembers) -> 500.
	collectionRepo.On("ListMembers", mock.Anything, "col-1", true).Return(nil, errors.New("db down"))
	_, err = svc.ReorderCollectionTemplates(ctx, &pb.ReorderCollectionTemplatesRequest{CollectionId: "col-1", TemplateIds: []string{"t-1"}})
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestReorderCollectionTemplates_TooMany(t *testing.T) {
	svc, collectionRepo, _, userRepo := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "owner-1")
	collectionRepo.On("GetByID", mock.Anything, "col-1").Return(activeCollection("col-1", "owner-1"), nil)
	userRepo.On("GetByID", mock.Anything, "owner-1").Return(activeUser("owner-1"), nil)

	ids := make([]string, 101)
	for i := range ids {
		ids[i] = "t"
	}
	_, err := svc.ReorderCollectionTemplates(ctx, &pb.ReorderCollectionTemplatesRequest{CollectionId: "col-1", TemplateIds: ids})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestAddTemplateToCollection_RepoErrors(t *testing.T) {
	svc, collectionRepo, templateRepo, userRepo := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "owner-1")
	collectionRepo.On("GetByID", mock.Anything, "col-1").Return(activeCollection("col-1", "owner-1"), nil)
	userRepo.On("GetByID", mock.Anything, "owner-1").Return(activeUser("owner-1"), nil)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return &models.Template{ID: "t-1", OwnerID: "someone", Visibility: "public"}, nil
	}
	collectionRepo.On("CountMembers", mock.Anything, "col-1").Return(int32(0), nil).Once()
	collectionRepo.On("GetMemberPosition", mock.Anything, "col-1", "t-1").Return(int32(-1), nil)
	collectionRepo.On("MaxMemberPosition", mock.Anything, "col-1").Return(int32(-1), errors.New("db down"))

	_, err := svc.AddTemplateToCollection(ctx, &pb.AddTemplateToCollectionRequest{CollectionId: "col-1", TemplateId: "t-1"})
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestListMyCollections_RepoError(t *testing.T) {
	svc, collectionRepo, _, _ := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "user-1")
	collectionRepo.On("ListByOwner", mock.Anything, "user-1").Return(nil, errors.New("db down"))

	_, err := svc.ListMyCollections(ctx, &pb.ListMyCollectionsRequest{})
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestListPublicCollections_RepoError(t *testing.T) {
	svc, collectionRepo, _, _ := newCollectionTestService(t)
	collectionRepo.On("ListPublic", mock.Anything, int32(0), int32(20), "newest").Return(nil, errors.New("db down"))

	_, err := svc.ListPublicCollections(context.Background(), &pb.ListPublicCollectionsRequest{})
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestGetCollection_OwnerRepoError(t *testing.T) {
	svc, collectionRepo, _, userRepo := newCollectionTestService(t)
	pub := activeCollection("col-pub", "owner-1")
	pub.Visibility = "public"
	collectionRepo.On("GetByID", mock.Anything, "col-pub").Return(pub, nil)
	userRepo.On("GetByID", mock.Anything, "owner-1").Return(nil, errors.New("db down"))

	_, err := svc.GetCollection(context.Background(), &pb.GetCollectionRequest{Id: "col-pub"})
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestGetCollection_MembersRepoError(t *testing.T) {
	svc, collectionRepo, _, userRepo := newCollectionTestService(t)
	pub := activeCollection("col-pub", "owner-1")
	pub.Visibility = "public"
	collectionRepo.On("GetByID", mock.Anything, "col-pub").Return(pub, nil)
	userRepo.On("GetByID", mock.Anything, "owner-1").Return(activeUser("owner-1"), nil)
	collectionRepo.On("ListMembers", mock.Anything, "col-pub", false).Return(nil, errors.New("db down"))

	_, err := svc.GetCollection(context.Background(), &pb.GetCollectionRequest{Id: "col-pub"})
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestListCollectionsContainingTemplate_RepoError(t *testing.T) {
	svc, collectionRepo, templateRepo, userRepo := newCollectionTestService(t)
	ctx := ContextWithUserID(context.Background(), "user-1")
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return &models.Template{ID: "t-1", OwnerID: "someone", Visibility: "public"}, nil
	}
	collectionRepo.On("ListContainingTemplate", mock.Anything, "user-1", "t-1").Return(nil, errors.New("db down"))
	userRepo.On("GetByID", mock.Anything, "user-1").Return(activeUser("user-1"), nil)

	_, err := svc.ListCollectionsContainingTemplate(ctx, &pb.ListCollectionsContainingTemplateRequest{TemplateId: "t-1"})
	assert.Equal(t, codes.Internal, status.Code(err))
}
