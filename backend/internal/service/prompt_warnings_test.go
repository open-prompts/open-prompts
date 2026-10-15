package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"
	"open-prompts/backend/internal/repository"
)

// newWarningsPromptService builds a PromptService wired with a review repo
// that returns the default taxonomy, so taxonomy validation and the
// exclude_warned filter can be exercised.
func newWarningsPromptService(t *testing.T) (*PromptService, *MockTemplateRepository, *MockTemplateVersionRepository, *MockReviewRepository) {
	t.Helper()
	mockTemplate := new(MockTemplateRepository)
	mockVersion := new(MockTemplateVersionRepository)
	mockAlias := &MockTemplateAliasRepository{}
	review := &MockReviewRepository{
		GetSettingFn: func(ctx context.Context, key string) (string, error) {
			if key == settingWarningTaxonomy {
				return `["mature","violence","profanity","sensitive"]`, nil
			}
			return "", repository.ErrSubmissionNotFound
		},
		GetUserGatingStateFn: func(ctx context.Context, userID string) (int32, int32, error) {
			return 0, 0, nil
		},
	}
	svc := NewPromptService(nil, mockTemplate, mockVersion, mockAlias, nil, nil, nil, nil)
	svc.SetReviewRepo(review)
	return svc, mockTemplate, mockVersion, review
}

func TestCreateTemplateWarnings(t *testing.T) {
	t.Run("valid taxonomy key accepted", func(t *testing.T) {
		svc, mockTemplate, mockVersion, _ := newWarningsPromptService(t)
		mockTemplate.CreateFunc = func(ctx context.Context, tmpl *models.Template) error {
			assert.Equal(t, []string{"mature"}, []string(tmpl.ContentWarnings))
			assert.Empty(t, tmpl.LockedWarnings)
			tmpl.ID = "tpl_1"
			return nil
		}
		mockVersion.CreateFunc = func(ctx context.Context, v *models.TemplateVersion) error {
			v.ID = 1
			return nil
		}
		resp, err := svc.CreateTemplate(ContextWithUserID(context.Background(), "user_1"), &pb.CreateTemplateRequest{
			Title:           "T",
			Content:         "C",
			ContentWarnings: []string{"mature"},
		})
		assert.NoError(t, err)
		assert.Equal(t, []string{"mature"}, resp.Template.ContentWarnings)
		assert.Empty(t, resp.Template.LockedWarnings)
	})

	t.Run("unknown taxonomy key rejected", func(t *testing.T) {
		svc, _, _, _ := newWarningsPromptService(t)
		_, err := svc.CreateTemplate(ContextWithUserID(context.Background(), "user_1"), &pb.CreateTemplateRequest{
			Title:           "T",
			Content:         "C",
			ContentWarnings: []string{"not_a_warning"},
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown content warning")
	})

	t.Run("empty array accepted", func(t *testing.T) {
		svc, mockTemplate, mockVersion, _ := newWarningsPromptService(t)
		mockTemplate.CreateFunc = func(ctx context.Context, tmpl *models.Template) error {
			assert.Empty(t, tmpl.ContentWarnings)
			tmpl.ID = "tpl_1"
			return nil
		}
		mockVersion.CreateFunc = func(ctx context.Context, v *models.TemplateVersion) error {
			v.ID = 1
			return nil
		}
		resp, err := svc.CreateTemplate(ContextWithUserID(context.Background(), "user_1"), &pb.CreateTemplateRequest{
			Title:   "T",
			Content: "C",
		})
		assert.NoError(t, err)
		assert.Empty(t, resp.Template.ContentWarnings)
	})
}

func TestUpdateTemplateWarningsLock(t *testing.T) {
	base := &models.Template{
		ID:              "tpl_1",
		OwnerID:         "user_1",
		Title:           "T",
		Visibility:      "public",
		ModerationStatus: "normal",
		ContentWarnings: []string{"mature", "violence"},
		LockedWarnings:  []string{"violence"},
	}

	t.Run("locked warning removed -> 403", func(t *testing.T) {
		svc, mockTemplate, _, _ := newWarningsPromptService(t)
		mockTemplate.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
			return base, nil
		}
		_, err := svc.UpdateTemplate(ContextWithUserID(context.Background(), "user_1"), &pb.UpdateTemplateRequest{
			TemplateId:      "tpl_1",
			Content:         "C",
			ContentWarnings: []string{"mature"}, // violence (locked) removed
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be removed")
	})

	t.Run("locked warning kept -> ok", func(t *testing.T) {
		svc, mockTemplate, mockVersion, _ := newWarningsPromptService(t)
		mockTemplate.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
			return base, nil
		}
		var updated *models.Template
		mockTemplate.UpdateFunc = func(ctx context.Context, tmpl *models.Template) error {
			updated = tmpl
			return nil
		}
		mockVersion.GetLatestFunc = func(ctx context.Context, templateID string) (*models.TemplateVersion, error) {
			return &models.TemplateVersion{ID: 1, Version: 1, Content: "C"}, nil
		}
		_, err := svc.UpdateTemplate(ContextWithUserID(context.Background(), "user_1"), &pb.UpdateTemplateRequest{
			TemplateId:      "tpl_1",
			Content:         "C",
			ContentWarnings: []string{"mature", "violence"},
		})
		assert.NoError(t, err)
		assert.NotNil(t, updated)
		assert.Equal(t, []string{"violence"}, []string(updated.LockedWarnings))
	})

	t.Run("self-declared removed -> ok", func(t *testing.T) {
		svc, mockTemplate, mockVersion, _ := newWarningsPromptService(t)
		mockTemplate.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
			return base, nil
		}
		var updated *models.Template
		mockTemplate.UpdateFunc = func(ctx context.Context, tmpl *models.Template) error {
			updated = tmpl
			return nil
		}
		mockVersion.GetLatestFunc = func(ctx context.Context, templateID string) (*models.TemplateVersion, error) {
			return &models.TemplateVersion{ID: 1, Version: 1, Content: "C"}, nil
		}
		_, err := svc.UpdateTemplate(ContextWithUserID(context.Background(), "user_1"), &pb.UpdateTemplateRequest{
			TemplateId:      "tpl_1",
			Content:         "C",
			ContentWarnings: []string{"violence"}, // mature (self-declared) removed
		})
		assert.NoError(t, err)
		assert.Equal(t, []string{"violence"}, []string(updated.ContentWarnings))
	})

	t.Run("unknown taxonomy key rejected", func(t *testing.T) {
		svc, mockTemplate, _, _ := newWarningsPromptService(t)
		// Use a template with no locked warnings so the taxonomy check (not
		// the lock check) is what rejects the bogus key.
		unlocked := &models.Template{
			ID:              "tpl_1",
			OwnerID:         "user_1",
			Title:           "T",
			Visibility:      "public",
			ModerationStatus: "normal",
			ContentWarnings: []string{"mature"},
			LockedWarnings:  nil,
		}
		mockTemplate.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
			return unlocked, nil
		}
		_, err := svc.UpdateTemplate(ContextWithUserID(context.Background(), "user_1"), &pb.UpdateTemplateRequest{
			TemplateId:      "tpl_1",
			Content:         "C",
			ContentWarnings: []string{"mature", "bogus"},
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown content warning")
	})
}

func TestListTemplatesExcludeWarned(t *testing.T) {
	t.Run("exclude_warned passed to public stream", func(t *testing.T) {
		svc, mockTemplate, mockVersion, _ := newWarningsPromptService(t)
		var gotFilters map[string]interface{}
		mockTemplate.ListFunc = func(ctx context.Context, l, o int, f map[string]interface{}) ([]*models.Template, error) {
			gotFilters = f
			return []*models.Template{}, nil
		}
		mockTemplate.CountTemplatesFunc = func(ctx context.Context, f map[string]interface{}) (int32, error) {
			return 0, nil
		}
		mockVersion.GetLatestFunc = func(ctx context.Context, templateID string) (*models.TemplateVersion, error) {
			return nil, repository.ErrSubmissionNotFound
		}
		_, err := svc.ListTemplates(context.Background(), &pb.ListTemplatesRequest{
			ExcludeWarned: true,
			Visibility:    pb.Visibility_VISIBILITY_PUBLIC,
		})
		assert.NoError(t, err)
		assert.NotNil(t, gotFilters)
		assert.Equal(t, true, gotFilters["exclude_warned"])
	})

	t.Run("exclude_warned not passed to private stream", func(t *testing.T) {
		svc, mockTemplate, mockVersion, _ := newWarningsPromptService(t)
		var gotFilters map[string]interface{}
		mockTemplate.ListFunc = func(ctx context.Context, l, o int, f map[string]interface{}) ([]*models.Template, error) {
			gotFilters = f
			return []*models.Template{}, nil
		}
		mockTemplate.CountTemplatesFunc = func(ctx context.Context, f map[string]interface{}) (int32, error) {
			return 0, nil
		}
		mockVersion.GetLatestFunc = func(ctx context.Context, templateID string) (*models.TemplateVersion, error) {
			return nil, repository.ErrSubmissionNotFound
		}
		_, err := svc.ListTemplates(ContextWithUserID(context.Background(), "user_1"), &pb.ListTemplatesRequest{
			ExcludeWarned: true,
			Visibility:    pb.Visibility_VISIBILITY_PRIVATE,
		})
		assert.NoError(t, err)
		assert.NotNil(t, gotFilters)
		_, ok := gotFilters["exclude_warned"]
		assert.False(t, ok)
	})
}

func TestTaxonomyFallback(t *testing.T) {
	t.Run("falls back to default on invalid JSON", func(t *testing.T) {
		svc, _, _, review := newWarningsPromptService(t)
		review.GetSettingFn = func(ctx context.Context, key string) (string, error) {
			return "not-json", nil
		}
		assert.Equal(t, defaultWarningTaxonomy, svc.taxonomy(context.Background()))
	})

	t.Run("falls back to default on empty", func(t *testing.T) {
		svc, _, _, review := newWarningsPromptService(t)
		review.GetSettingFn = func(ctx context.Context, key string) (string, error) {
			return "", nil
		}
		assert.Equal(t, defaultWarningTaxonomy, svc.taxonomy(context.Background()))
	})

	t.Run("returns custom taxonomy", func(t *testing.T) {
		svc, _, _, review := newWarningsPromptService(t)
		review.GetSettingFn = func(ctx context.Context, key string) (string, error) {
			return `["custom_a","custom_b"]`, nil
		}
		assert.Equal(t, []string{"custom_a", "custom_b"}, svc.taxonomy(context.Background()))
	})
}

var _ = mock.Anything // keep testify/mock import for shared mock helpers