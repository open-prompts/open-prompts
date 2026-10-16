package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func relatedTestTemplate(ownerID string) *models.Template {
	return &models.Template{
		ID:         "tpl-1",
		OwnerID:    ownerID,
		Visibility: "public",
		Category:   sql.NullString{String: "Customer Support", Valid: true},
		Tags:       []string{"writing", "email"},
		Language:   "en",
	}
}

func newRelatedTestService(t *testing.T) (*PromptService, *MockTemplateRepository, *fakeRedis) {
	t.Helper()
	templateRepo := new(MockTemplateRepository)
	redis := newFakeRedis()
	svc := NewPromptService(nil, templateRepo, nil, nil, nil, nil, nil, redis)
	return svc, templateRepo, redis
}

func TestGetRelatedTemplates_VisibilityGate(t *testing.T) {
	svc, templateRepo, _ := newRelatedTestService(t)

	// Nonexistent template -> 404.
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return nil, errors.New("template not found")
	}
	_, err := svc.GetRelatedTemplates(context.Background(), &pb.GetRelatedTemplatesRequest{TemplateId: "missing"})
	assert.Equal(t, codes.NotFound, status.Code(err))

	// Private template -> 404 for owner and non-owner.
	for _, owner := range []string{"owner-1", "other"} {
		templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
			return &models.Template{ID: "tpl-1", OwnerID: "owner-1", Visibility: "private"}, nil
		}
		ctx := ContextWithUserID(context.Background(), owner)
		_, err := svc.GetRelatedTemplates(ctx, &pb.GetRelatedTemplatesRequest{TemplateId: "tpl-1"})
		assert.Equal(t, codes.NotFound, status.Code(err), "private template should 404 for %s", owner)
	}
}

func TestGetRelatedTemplates_LimitClamping(t *testing.T) {
	svc, templateRepo, _ := newRelatedTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return relatedTestTemplate("owner-1"), nil
	}
	var gotLimit int
	templateRepo.ListRelatedFunc = func(ctx context.Context, templateID string, tpl *models.Template, uid string, limit int) ([]*models.Template, error) {
		gotLimit = limit
		return []*models.Template{}, nil
	}

	// Default (0) -> 8.
	_, err := svc.GetRelatedTemplates(context.Background(), &pb.GetRelatedTemplatesRequest{TemplateId: "tpl-1"})
	require.NoError(t, err)
	assert.Equal(t, 8, gotLimit)

	// limit=100 -> clamped to 8.
	_, err = svc.GetRelatedTemplates(context.Background(), &pb.GetRelatedTemplatesRequest{TemplateId: "tpl-1", Limit: 100})
	require.NoError(t, err)
	assert.Equal(t, 8, gotLimit)

	// limit=1 -> clamped to 3.
	_, err = svc.GetRelatedTemplates(context.Background(), &pb.GetRelatedTemplatesRequest{TemplateId: "tpl-1", Limit: 1})
	require.NoError(t, err)
	assert.Equal(t, 3, gotLimit)
}

func TestGetRelatedTemplates_BasisDerivation(t *testing.T) {
	svc, templateRepo, redis := newRelatedTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return relatedTestTemplate("owner-1"), nil
	}

	// Category basis: top item shares category.
	templateRepo.ListRelatedFunc = func(ctx context.Context, templateID string, tpl *models.Template, uid string, limit int) ([]*models.Template, error) {
		return []*models.Template{
			{ID: "a", Category: sql.NullString{String: "Customer Support", Valid: true}, Tags: []string{"other"}},
			{ID: "b", Category: sql.NullString{String: "Other", Valid: true}, Tags: []string{"writing"}},
			{ID: "c", Category: sql.NullString{String: "Other", Valid: true}, Tags: []string{"x"}},
		}, nil
	}
	resp, err := svc.GetRelatedTemplates(context.Background(), &pb.GetRelatedTemplatesRequest{TemplateId: "tpl-1"})
	require.NoError(t, err)
	assert.Equal(t, "category", resp.Basis)
	assert.Len(t, resp.Items, 3)

	// Tag basis: top item shares a tag but not category.
	redis.data = map[string]string{} // clear cache
	templateRepo.ListRelatedFunc = func(ctx context.Context, templateID string, tpl *models.Template, uid string, limit int) ([]*models.Template, error) {
		return []*models.Template{
			{ID: "a", Category: sql.NullString{String: "Other", Valid: true}, Tags: []string{"writing"}},
			{ID: "b", Category: sql.NullString{String: "Other", Valid: true}, Tags: []string{"x"}},
			{ID: "c", Category: sql.NullString{String: "Other", Valid: true}, Tags: []string{"y"}},
		}, nil
	}
	resp, err = svc.GetRelatedTemplates(context.Background(), &pb.GetRelatedTemplatesRequest{TemplateId: "tpl-1"})
	require.NoError(t, err)
	assert.Equal(t, "tag", resp.Basis)

	// Generic basis: no shared category or tag.
	redis.data = map[string]string{} // clear cache
	templateRepo.ListRelatedFunc = func(ctx context.Context, templateID string, tpl *models.Template, uid string, limit int) ([]*models.Template, error) {
		return []*models.Template{
			{ID: "a", Category: sql.NullString{String: "Other", Valid: true}, Tags: []string{"x"}},
			{ID: "b", Category: sql.NullString{String: "Other", Valid: true}, Tags: []string{"y"}},
			{ID: "c", Category: sql.NullString{String: "Other", Valid: true}, Tags: []string{"z"}},
		}, nil
	}
	resp, err = svc.GetRelatedTemplates(context.Background(), &pb.GetRelatedTemplatesRequest{TemplateId: "tpl-1"})
	require.NoError(t, err)
	assert.Equal(t, "generic", resp.Basis)
}

func TestGetRelatedTemplates_FewerThanThree(t *testing.T) {
	svc, templateRepo, _ := newRelatedTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return relatedTestTemplate("owner-1"), nil
	}
	templateRepo.ListRelatedFunc = func(ctx context.Context, templateID string, tpl *models.Template, uid string, limit int) ([]*models.Template, error) {
		return []*models.Template{
			{ID: "a", Category: sql.NullString{String: "Customer Support", Valid: true}},
			{ID: "b", Category: sql.NullString{String: "Customer Support", Valid: true}},
		}, nil
	}
	resp, err := svc.GetRelatedTemplates(context.Background(), &pb.GetRelatedTemplatesRequest{TemplateId: "tpl-1"})
	require.NoError(t, err)
	assert.Empty(t, resp.Items)
	assert.Equal(t, "generic", resp.Basis)
}

func TestGetRelatedTemplates_CacheHit(t *testing.T) {
	svc, templateRepo, redis := newRelatedTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return relatedTestTemplate("owner-1"), nil
	}
	// Pre-populate the cache so the repo is never called.
	cached := `{"items":[{"id":"a"},{"id":"b"},{"id":"c"}],"basis":"category"}`
	redis.data["related:v1:tpl-1:8"] = cached
	called := false
	templateRepo.ListRelatedFunc = func(ctx context.Context, templateID string, tpl *models.Template, uid string, limit int) ([]*models.Template, error) {
		called = true
		return nil, nil
	}
	resp, err := svc.GetRelatedTemplates(context.Background(), &pb.GetRelatedTemplatesRequest{TemplateId: "tpl-1"})
	require.NoError(t, err)
	assert.False(t, called, "repo should not be called on cache hit")
	assert.Equal(t, "category", resp.Basis)
	assert.Len(t, resp.Items, 3)
}

func TestGetRelatedTemplates_CacheWriteFailureSwallowed(t *testing.T) {
	svc, templateRepo, redis := newRelatedTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return relatedTestTemplate("owner-1"), nil
	}
	templateRepo.ListRelatedFunc = func(ctx context.Context, templateID string, tpl *models.Template, uid string, limit int) ([]*models.Template, error) {
		return []*models.Template{
			{ID: "a", Category: sql.NullString{String: "Customer Support", Valid: true}},
			{ID: "b", Category: sql.NullString{String: "Customer Support", Valid: true}},
			{ID: "c", Category: sql.NullString{String: "Customer Support", Valid: true}},
		}, nil
	}
	redis.fail = true // Set fails
	resp, err := svc.GetRelatedTemplates(context.Background(), &pb.GetRelatedTemplatesRequest{TemplateId: "tpl-1"})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 3)
}

func TestGetRelatedTemplates_RepoError(t *testing.T) {
	svc, templateRepo, _ := newRelatedTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return relatedTestTemplate("owner-1"), nil
	}
	templateRepo.ListRelatedFunc = func(ctx context.Context, templateID string, tpl *models.Template, uid string, limit int) ([]*models.Template, error) {
		return nil, errors.New("db down")
	}
	_, err := svc.GetRelatedTemplates(context.Background(), &pb.GetRelatedTemplatesRequest{TemplateId: "tpl-1"})
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestListMoreFromAuthor_VisibilityGate(t *testing.T) {
	svc, templateRepo, _ := newRelatedTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return &models.Template{ID: "tpl-1", OwnerID: "owner-1", Visibility: "private"}, nil
	}
	_, err := svc.ListMoreFromAuthor(context.Background(), &pb.ListMoreFromAuthorRequest{TemplateId: "tpl-1"})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestListMoreFromAuthor_LimitClamping(t *testing.T) {
	svc, templateRepo, _ := newRelatedTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return relatedTestTemplate("owner-1"), nil
	}
	var gotLimit int
	templateRepo.ListMoreFromAuthorFunc = func(ctx context.Context, templateID, ownerID, uid string, limit int) ([]*models.Template, error) {
		gotLimit = limit
		return []*models.Template{}, nil
	}
	// Default -> 4.
	_, err := svc.ListMoreFromAuthor(context.Background(), &pb.ListMoreFromAuthorRequest{TemplateId: "tpl-1"})
	require.NoError(t, err)
	assert.Equal(t, 4, gotLimit)
	// limit=100 -> 4.
	_, err = svc.ListMoreFromAuthor(context.Background(), &pb.ListMoreFromAuthorRequest{TemplateId: "tpl-1", Limit: 100})
	require.NoError(t, err)
	assert.Equal(t, 4, gotLimit)
	// limit=0 -> 4 (default).
	_, err = svc.ListMoreFromAuthor(context.Background(), &pb.ListMoreFromAuthorRequest{TemplateId: "tpl-1", Limit: 0})
	require.NoError(t, err)
	assert.Equal(t, 4, gotLimit)
}

func TestListMoreFromAuthor_ReturnsItems(t *testing.T) {
	svc, templateRepo, _ := newRelatedTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return relatedTestTemplate("owner-1"), nil
	}
	templateRepo.ListMoreFromAuthorFunc = func(ctx context.Context, templateID, ownerID, uid string, limit int) ([]*models.Template, error) {
		assert.Equal(t, "owner-1", ownerID)
		return []*models.Template{
			{ID: "a", OwnerID: "owner-1", Visibility: "public"},
			{ID: "b", OwnerID: "owner-1", Visibility: "public"},
		}, nil
	}
	resp, err := svc.ListMoreFromAuthor(context.Background(), &pb.ListMoreFromAuthorRequest{TemplateId: "tpl-1"})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 2)
	assert.Equal(t, "a", resp.Items[0].Id)
}

func TestListMoreFromAuthor_RepoError(t *testing.T) {
	svc, templateRepo, _ := newRelatedTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return relatedTestTemplate("owner-1"), nil
	}
	templateRepo.ListMoreFromAuthorFunc = func(ctx context.Context, templateID, ownerID, uid string, limit int) ([]*models.Template, error) {
		return nil, errors.New("db down")
	}
	_, err := svc.ListMoreFromAuthor(context.Background(), &pb.ListMoreFromAuthorRequest{TemplateId: "tpl-1"})
	assert.Equal(t, codes.Internal, status.Code(err))
}
