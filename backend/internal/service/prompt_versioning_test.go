package service

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// newVersioningTestService builds a PromptService wired for UpdateTemplate /
// ListTemplateVersions tests. latest is returned by GetLatest; created (if
// non-nil) captures what the repository Create call persists.
func newVersioningTestService(t *testing.T, latest *models.TemplateVersion, created **models.TemplateVersion) *PromptService {
	t.Helper()
	mockTemplateRepo := &MockTemplateRepository{
		GetFunc: func(ctx context.Context, id string, currentUserID string) (*models.Template, error) {
			return &models.Template{ID: id, OwnerID: "owner_1", Title: "T", Visibility: "public"}, nil
		},
		UpdateFunc: func(ctx context.Context, tpl *models.Template) error { return nil },
	}
	mockVersionRepo := &MockTemplateVersionRepository{
		GetLatestFunc: func(ctx context.Context, tid string) (*models.TemplateVersion, error) {
			if latest == nil {
				return nil, sql.ErrNoRows
			}
			return latest, nil
		},
		CreateFunc: func(ctx context.Context, v *models.TemplateVersion) error {
			v.ID = 99
			if created != nil {
				*created = v
			}
			return nil
		},
	}
	mockAliasRepo := &MockTemplateAliasRepository{
		UpsertFunc: func(ctx context.Context, alias *models.TemplateAlias) error { return nil },
	}
	return NewPromptService(nil, mockTemplateRepo, mockVersionRepo, mockAliasRepo, nil, nil, nil, nil)
}

func TestUpdateTemplateChangeNote(t *testing.T) {
	latest := &models.TemplateVersion{ID: 1, TemplateID: "t1", Version: 1, Content: "old content"}

	t.Run("NotePersistedOnContentChange", func(t *testing.T) {
		var created *models.TemplateVersion
		svc := newVersioningTestService(t, latest, &created)
		resp, err := svc.UpdateTemplate(context.Background(), &pb.UpdateTemplateRequest{
			TemplateId: "t1",
			OwnerId:    "owner_1",
			Content:    "new content",
			ChangeNote: "first revision",
		})
		assert.NoError(t, err)
		assert.NotNil(t, created)
		assert.Equal(t, "first revision", created.ChangeNote.String)
		assert.True(t, created.ChangeNote.Valid)
		assert.Equal(t, int32(2), created.Version)
		assert.Equal(t, "first revision", resp.NewVersion.ChangeNote)
	})

	t.Run("NoteLength200Ok", func(t *testing.T) {
		var created *models.TemplateVersion
		svc := newVersioningTestService(t, latest, &created)
		note := strings.Repeat("x", 200)
		_, err := svc.UpdateTemplate(context.Background(), &pb.UpdateTemplateRequest{
			TemplateId: "t1",
			OwnerId:    "owner_1",
			Content:    "new content",
			ChangeNote: note,
		})
		assert.NoError(t, err)
		assert.Equal(t, 200, len(created.ChangeNote.String))
	})

	t.Run("NoteLength201Rejected", func(t *testing.T) {
		var created *models.TemplateVersion
		svc := newVersioningTestService(t, latest, &created)
		note := strings.Repeat("x", 201)
		_, err := svc.UpdateTemplate(context.Background(), &pb.UpdateTemplateRequest{
			TemplateId: "t1",
			OwnerId:    "owner_1",
			Content:    "new content",
			ChangeNote: note,
		})
		assert.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
		assert.Nil(t, created) // no version written
	})

	t.Run("NoteIgnoredWhenContentUnchanged", func(t *testing.T) {
		var created *models.TemplateVersion
		latestWithNote := &models.TemplateVersion{
			ID: 1, TemplateID: "t1", Version: 1, Content: "same content",
			ChangeNote: sql.NullString{String: "original note", Valid: true},
		}
		svc := newVersioningTestService(t, latestWithNote, &created)
		resp, err := svc.UpdateTemplate(context.Background(), &pb.UpdateTemplateRequest{
			TemplateId: "t1",
			OwnerId:    "owner_1",
			Content:    "same content",
			ChangeNote: "should be ignored",
		})
		assert.NoError(t, err)
		assert.Nil(t, created) // no new version
		assert.Equal(t, int32(1), resp.NewVersion.Version)
		assert.Equal(t, "original note", resp.NewVersion.ChangeNote) // untouched
	})

	t.Run("EmptyNoteStoredAsNull", func(t *testing.T) {
		var created *models.TemplateVersion
		svc := newVersioningTestService(t, latest, &created)
		_, err := svc.UpdateTemplate(context.Background(), &pb.UpdateTemplateRequest{
			TemplateId: "t1",
			OwnerId:    "owner_1",
			Content:    "new content",
		})
		assert.NoError(t, err)
		assert.False(t, created.ChangeNote.Valid)
		assert.Equal(t, "", respChangeNote(created))
	})

	t.Run("RestoreNoteCanonicalString", func(t *testing.T) {
		// Latest is v2; restoring means sending v1's (different) content.
		latestV2 := &models.TemplateVersion{ID: 2, TemplateID: "t1", Version: 2, Content: "version two content"}
		var created *models.TemplateVersion
		svc := newVersioningTestService(t, latestV2, &created)
		_, err := svc.UpdateTemplate(context.Background(), &pb.UpdateTemplateRequest{
			TemplateId: "t1",
			OwnerId:    "owner_1",
			Content:    "old content", // v1's content
			ChangeNote: "Restored from v1",
		})
		assert.NoError(t, err)
		assert.Equal(t, "Restored from v1", created.ChangeNote.String)
		assert.Equal(t, int32(3), created.Version)
	})
}

func respChangeNote(v *models.TemplateVersion) string {
	svc := &PromptService{}
	return svc.versionModelToProto(v).ChangeNote
}

func TestVersionModelToProtoMapping(t *testing.T) {
	svc := &PromptService{}

	t.Run("NullNoteMapsToEmptyString", func(t *testing.T) {
		m := &models.TemplateVersion{
			ID: 7, TemplateID: "t1", Version: 3, Content: "c",
			ChangeNote:  sql.NullString{Valid: false},
			PromptCount: 5,
		}
		p := svc.versionModelToProto(m)
		assert.Equal(t, "", p.ChangeNote)
		assert.Equal(t, int32(5), p.PromptCount)
		assert.Equal(t, int32(7), p.Id)
		assert.Equal(t, int32(3), p.Version)
	})

	t.Run("ValidNoteMapped", func(t *testing.T) {
		m := &models.TemplateVersion{
			ChangeNote: sql.NullString{String: "note", Valid: true},
		}
		p := svc.versionModelToProto(m)
		assert.Equal(t, "note", p.ChangeNote)
	})

	t.Run("NilModelReturnsNil", func(t *testing.T) {
		assert.Nil(t, svc.versionModelToProto(nil))
	})
}

func TestListTemplateVersionsMapsCounts(t *testing.T) {
	mockVersionRepo := &MockTemplateVersionRepository{}
	mockVersionRepo.On("List", mock.Anything, 10, 0, "t1").Return([]*models.TemplateVersion{
		{ID: 2, Version: 2, PromptCount: 0, ChangeNote: sql.NullString{String: "second", Valid: true}},
		{ID: 1, Version: 1, PromptCount: 3, ChangeNote: sql.NullString{Valid: false}},
	}, nil).Once()

	svc := NewPromptService(nil, nil, mockVersionRepo, nil, nil, nil, nil, nil)
	resp, err := svc.ListTemplateVersions(context.Background(), &pb.ListTemplateVersionsRequest{
		TemplateId: "t1",
		PageSize:   10,
	})
	assert.NoError(t, err)
	assert.Len(t, resp.Versions, 2)
	assert.Equal(t, int32(0), resp.Versions[0].PromptCount)
	assert.Equal(t, "second", resp.Versions[0].ChangeNote)
	assert.Equal(t, int32(3), resp.Versions[1].PromptCount)
	assert.Equal(t, "", resp.Versions[1].ChangeNote)
}
