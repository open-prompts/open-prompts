package service

import (
	"context"
	"testing"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeKeyword(t *testing.T) {
	assert.Equal(t, "", normalizeKeyword(""))
	assert.Equal(t, "", normalizeKeyword("   "))
	assert.Equal(t, "hello world", normalizeKeyword("  hello \t world  "))
	// CJK-safe 100-rune cap (each CJK char is 3 bytes).
	long := ""
	for i := 0; i < 150; i++ {
		long += "好"
	}
	got := normalizeKeyword(long)
	assert.Equal(t, 100, len([]rune(got)))
	// ASCII cap
	longASCII := ""
	for i := 0; i < 150; i++ {
		longASCII += "a"
	}
	assert.Equal(t, 100, len(normalizeKeyword(longASCII)))
}

func TestNormalizeSort(t *testing.T) {
	assert.Equal(t, "newest", normalizeSort(""))
	assert.Equal(t, "newest", normalizeSort("newest"))
	assert.Equal(t, "most_liked", normalizeSort("most_liked"))
	assert.Equal(t, "most_favorited", normalizeSort("most_favorited"))
	assert.Equal(t, "trending", normalizeSort("trending"))
	assert.Equal(t, "newest", normalizeSort("bogus"))
	assert.Equal(t, "newest", normalizeSort("NEWEST"))
	assert.Equal(t, "newest", normalizeSort("; DROP TABLE templates"))
}

func TestListTemplatesSearchFilters(t *testing.T) {
	mockTemplateRepo := new(MockTemplateRepository)
	svc := NewPromptService(new(MockPromptRepository), mockTemplateRepo, new(MockTemplateVersionRepository), &MockTemplateAliasRepository{}, nil, nil, nil, nil)

	var capturedFilters map[string]interface{}
	var countFilters map[string]interface{}
	mockTemplateRepo.ListFunc = func(ctx context.Context, limit, offset int, filters map[string]interface{}) ([]*models.Template, error) {
		capturedFilters = filters
		return []*models.Template{{ID: "t_1", OwnerID: "user_1", Visibility: "public"}}, nil
	}
	mockTemplateRepo.CountTemplatesFunc = func(ctx context.Context, filters map[string]interface{}) (int32, error) {
		countFilters = filters
		return 42, nil
	}

	t.Run("AnonymousKeywordAndSort", func(t *testing.T) {
		resp, err := svc.ListTemplates(context.Background(), &pb.ListTemplatesRequest{
			Keyword: "  machine   learning ",
			Sort:    "most_liked",
		})
		assert.NoError(t, err)
		assert.Equal(t, "machine learning", capturedFilters["keyword"])
		assert.Equal(t, "most_liked", capturedFilters["sort"])
		assert.Equal(t, "public", capturedFilters["visibility"])
		assert.Equal(t, int32(42), resp.TotalCount)
		assert.Equal(t, int32(0), resp.PrivateTotalCount)
		// Count filters must not include current_user_id
		_, hasUser := countFilters["current_user_id"]
		assert.False(t, hasUser)
		assert.Equal(t, "machine learning", countFilters["keyword"])
	})

	t.Run("InvalidSortFallsBackToNewest", func(t *testing.T) {
		_, err := svc.ListTemplates(context.Background(), &pb.ListTemplatesRequest{Sort: "hacker"})
		assert.NoError(t, err)
		assert.Equal(t, "newest", capturedFilters["sort"])
	})

	t.Run("EmptyKeywordNotSet", func(t *testing.T) {
		_, err := svc.ListTemplates(context.Background(), &pb.ListTemplatesRequest{})
		assert.NoError(t, err)
		_, hasKeyword := capturedFilters["keyword"]
		assert.False(t, hasKeyword)
		assert.Equal(t, "newest", capturedFilters["sort"])
	})

	t.Run("CountOnlyOnFirstPage", func(t *testing.T) {
		countCalled := false
		mockTemplateRepo.CountTemplatesFunc = func(ctx context.Context, filters map[string]interface{}) (int32, error) {
			countCalled = true
			return 7, nil
		}
		resp, err := svc.ListTemplates(context.Background(), &pb.ListTemplatesRequest{PageToken: "10"})
		assert.NoError(t, err)
		assert.False(t, countCalled)
		assert.Equal(t, int32(0), resp.TotalCount)
	})

	t.Run("AuthenticatedMixedPathCounts", func(t *testing.T) {
		countCalls := 0
		mockTemplateRepo.CountTemplatesFunc = func(ctx context.Context, filters map[string]interface{}) (int32, error) {
			countCalls++
			if filters["visibility"] == "private" {
				return 3, nil
			}
			return 9, nil
		}
		resp, err := svc.ListTemplates(ContextWithUserID(context.Background(), "user_1"), &pb.ListTemplatesRequest{Keyword: "go"})
		assert.NoError(t, err)
		assert.Equal(t, 2, countCalls)
		assert.Equal(t, int32(9), resp.TotalCount)
		assert.Equal(t, int32(3), resp.PrivateTotalCount)
		assert.Equal(t, "go", capturedFilters["keyword"])
	})

	t.Run("MyLikesPathAppliesSearch", func(t *testing.T) {
		_, err := svc.ListTemplates(ContextWithUserID(context.Background(), "user_1"), &pb.ListTemplatesRequest{
			MyLikes: true,
			Keyword: "test",
			Sort:    "trending",
		})
		assert.NoError(t, err)
		assert.Equal(t, true, capturedFilters["my_likes"])
		assert.Equal(t, "test", capturedFilters["keyword"])
		assert.Equal(t, "trending", capturedFilters["sort"])
	})

	t.Run("SpecificVisibilityPathAppliesSearch", func(t *testing.T) {
		_, err := svc.ListTemplates(ContextWithUserID(context.Background(), "user_1"), &pb.ListTemplatesRequest{
			Visibility: pb.Visibility_VISIBILITY_PRIVATE,
			Keyword:    "secret",
		})
		assert.NoError(t, err)
		assert.Equal(t, "private", capturedFilters["visibility"])
		assert.Equal(t, "secret", capturedFilters["keyword"])
		assert.Equal(t, "newest", capturedFilters["sort"])
	})

	t.Run("CountErrorFailsOpen", func(t *testing.T) {
		mockTemplateRepo.CountTemplatesFunc = func(ctx context.Context, filters map[string]interface{}) (int32, error) {
			return 0, assert.AnError
		}
		resp, err := svc.ListTemplates(context.Background(), &pb.ListTemplatesRequest{})
		assert.NoError(t, err)
		assert.Equal(t, int32(0), resp.TotalCount)
	})
}
