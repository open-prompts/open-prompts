package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"
	"open-prompts/backend/internal/repository"
)

// ---- MockReviewRepository ----

// MockReviewRepository is a hand-written mock (func-based, matching the
// prompt-service mock style) implementing repository.ReviewRepository.
type MockReviewRepository struct {
	CreateSubmissionFn    func(ctx context.Context, s *models.ReviewSubmission) error
	GetSubmissionFn       func(ctx context.Context, id string) (*models.ReviewSubmission, error)
	GetLatestSubmissionFn func(ctx context.Context, templateID string) (*models.ReviewSubmission, error)
	ListQueueFn           func(ctx context.Context, f repository.ReviewQueueFilters) ([]*models.ReviewQueueRow, int64, error)
	GetSubmissionDetailFn func(ctx context.Context, id string) (*models.ReviewSubmissionDetail, error)
	ListChainFn           func(ctx context.Context, templateID string) ([]*models.ReviewSubmission, error)
	DecideSubmissionFn    func(ctx context.Context, id, decidedStatus, decidedBy string, reasonCode, note *string) (*models.ReviewSubmission, error)
	GetMetricsFn          func(ctx context.Context) (*models.ReviewMetrics, error)
	GetSettingsFn         func(ctx context.Context) ([]*models.PlatformSetting, error)
	GetSettingFn          func(ctx context.Context, key string) (string, error)
	UpsertSettingFn       func(ctx context.Context, key, value string) error
	GetUserGatingStateFn  func(ctx context.Context, userID string) (int32, int32, error)
	IncrementApprovedFn   func(ctx context.Context, userID string) error
	DecrementGatedFn      func(ctx context.Context, userID string) error
	SetGatedRemainingFn   func(ctx context.Context, userID string, value int32) error
}

func (m *MockReviewRepository) CreateSubmission(ctx context.Context, s *models.ReviewSubmission) error {
	if m.CreateSubmissionFn != nil {
		return m.CreateSubmissionFn(ctx, s)
	}
	return nil
}

func (m *MockReviewRepository) GetSubmission(ctx context.Context, id string) (*models.ReviewSubmission, error) {
	if m.GetSubmissionFn != nil {
		return m.GetSubmissionFn(ctx, id)
	}
	return nil, repository.ErrSubmissionNotFound
}

func (m *MockReviewRepository) GetLatestSubmission(ctx context.Context, templateID string) (*models.ReviewSubmission, error) {
	if m.GetLatestSubmissionFn != nil {
		return m.GetLatestSubmissionFn(ctx, templateID)
	}
	return nil, repository.ErrSubmissionNotFound
}

func (m *MockReviewRepository) ListQueue(ctx context.Context, f repository.ReviewQueueFilters) ([]*models.ReviewQueueRow, int64, error) {
	if m.ListQueueFn != nil {
		return m.ListQueueFn(ctx, f)
	}
	return nil, 0, nil
}

func (m *MockReviewRepository) GetSubmissionDetail(ctx context.Context, id string) (*models.ReviewSubmissionDetail, error) {
	if m.GetSubmissionDetailFn != nil {
		return m.GetSubmissionDetailFn(ctx, id)
	}
	return nil, repository.ErrSubmissionNotFound
}

func (m *MockReviewRepository) ListChain(ctx context.Context, templateID string) ([]*models.ReviewSubmission, error) {
	if m.ListChainFn != nil {
		return m.ListChainFn(ctx, templateID)
	}
	return nil, nil
}

func (m *MockReviewRepository) DecideSubmission(ctx context.Context, id, decidedStatus, decidedBy string, reasonCode, note *string) (*models.ReviewSubmission, error) {
	if m.DecideSubmissionFn != nil {
		return m.DecideSubmissionFn(ctx, id, decidedStatus, decidedBy, reasonCode, note)
	}
	return nil, repository.ErrSubmissionNotFound
}

func (m *MockReviewRepository) GetMetrics(ctx context.Context) (*models.ReviewMetrics, error) {
	if m.GetMetricsFn != nil {
		return m.GetMetricsFn(ctx)
	}
	return &models.ReviewMetrics{}, nil
}

func (m *MockReviewRepository) GetSettings(ctx context.Context) ([]*models.PlatformSetting, error) {
	if m.GetSettingsFn != nil {
		return m.GetSettingsFn(ctx)
	}
	return nil, nil
}

func (m *MockReviewRepository) GetSetting(ctx context.Context, key string) (string, error) {
	if m.GetSettingFn != nil {
		return m.GetSettingFn(ctx, key)
	}
	return "", errors.New("setting not found")
}

func (m *MockReviewRepository) UpsertSetting(ctx context.Context, key, value string) error {
	if m.UpsertSettingFn != nil {
		return m.UpsertSettingFn(ctx, key, value)
	}
	return nil
}

func (m *MockReviewRepository) GetUserGatingState(ctx context.Context, userID string) (int32, int32, error) {
	if m.GetUserGatingStateFn != nil {
		return m.GetUserGatingStateFn(ctx, userID)
	}
	return 0, 0, nil
}

func (m *MockReviewRepository) IncrementApprovedCount(ctx context.Context, userID string) error {
	if m.IncrementApprovedFn != nil {
		return m.IncrementApprovedFn(ctx, userID)
	}
	return nil
}

func (m *MockReviewRepository) DecrementGatedRemaining(ctx context.Context, userID string) error {
	if m.DecrementGatedFn != nil {
		return m.DecrementGatedFn(ctx, userID)
	}
	return nil
}

func (m *MockReviewRepository) SetGatedRemaining(ctx context.Context, userID string, value int32) error {
	if m.SetGatedRemainingFn != nil {
		return m.SetGatedRemainingFn(ctx, userID, value)
	}
	return nil
}

// ---- helpers ----

// newReviewTestService builds an AdminService with a review repo wired.
func newReviewTestService(t *testing.T) (*AdminService, *MockUserRepository, *MockAdminRepository, *MockReviewRepository) {
	t.Helper()
	svc, mockUser, mockAdmin, _, _ := newAdminTestService(t)
	mockReview := new(MockReviewRepository)
	svc.SetReviewRepo(mockReview)
	return svc, mockUser, mockAdmin, mockReview
}

func makePendingSubmission(id, templateID, creatorID string, attempt int32) *models.ReviewSubmission {
	return &models.ReviewSubmission{
		ID:            id,
		TemplateID:    templateID,
		CreatorID:     creatorID,
		AttemptNumber: attempt,
		Status:        "pending",
	}
}

// ---- isGated ----

func TestIsGated(t *testing.T) {
	tests := []struct {
		name        string
		approved    int32
		gatedRemain int32
		threshold   string
		repoErr     error
		want        bool
	}{
		{name: "new creator (0 approved) is gated", approved: 0, gatedRemain: 0, threshold: "3", want: true},
		{name: "approved below threshold is gated", approved: 2, gatedRemain: 0, threshold: "3", want: true},
		{name: "approved at threshold is exempt", approved: 3, gatedRemain: 0, threshold: "3", want: false},
		{name: "approved above threshold is exempt", approved: 5, gatedRemain: 0, threshold: "3", want: false},
		{name: "re-gated creator is gated even when established", approved: 10, gatedRemain: 2, threshold: "3", want: true},
		{name: "repo error fails closed", approved: 3, gatedRemain: 0, threshold: "3", repoErr: errors.New("db down"), want: true},
		{name: "missing setting falls back to default 3", approved: 3, gatedRemain: 0, threshold: "", want: false},
		{name: "threshold 0 disables gating", approved: 0, gatedRemain: 5, threshold: "0", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := NewPromptService(nil, nil, nil, nil, nil, nil, nil, nil)
			mockReview := new(MockReviewRepository)
			mockReview.GetUserGatingStateFn = func(ctx context.Context, userID string) (int32, int32, error) {
				return tt.approved, tt.gatedRemain, tt.repoErr
			}
			if tt.threshold != "" {
				mockReview.GetSettingFn = func(ctx context.Context, key string) (string, error) {
					if key == settingApprovalThreshold {
						return tt.threshold, nil
					}
					return "", errors.New("not found")
				}
			}
			ps.SetReviewRepo(mockReview)
			require.Equal(t, tt.want, ps.isGated(context.Background(), "user_1"))
		})
	}
}

func TestIsGatedNoRepo(t *testing.T) {
	ps := NewPromptService(nil, nil, nil, nil, nil, nil, nil, nil)
	require.False(t, ps.isGated(context.Background(), "user_1"))
}

func TestGetSettingIntFallbacks(t *testing.T) {
	ps := NewPromptService(nil, nil, nil, nil, nil, nil, nil, nil)
	// No repo: built-in defaults.
	require.Equal(t, 3, ps.getSettingInt(context.Background(), settingApprovalThreshold))
	require.Equal(t, 3, ps.getSettingInt(context.Background(), settingRegateCount))
	require.Equal(t, 48, ps.getSettingInt(context.Background(), settingAgingHours))

	mockReview := new(MockReviewRepository)
	mockReview.GetSettingFn = func(ctx context.Context, key string) (string, error) {
		return "not-a-number", nil
	}
	ps.SetReviewRepo(mockReview)
	// Non-numeric value falls back to the default.
	require.Equal(t, 3, ps.getSettingInt(context.Background(), settingApprovalThreshold))
}

func TestInvalidateSettingsCache(t *testing.T) {
	ps := NewPromptService(nil, nil, nil, nil, nil, nil, nil, nil)
	mockReview := new(MockReviewRepository)
	calls := 0
	mockReview.GetSettingFn = func(ctx context.Context, key string) (string, error) {
		calls++
		return "7", nil
	}
	ps.SetReviewRepo(mockReview)
	require.Equal(t, 7, ps.getSettingInt(context.Background(), settingApprovalThreshold))
	require.Equal(t, 7, ps.getSettingInt(context.Background(), settingApprovalThreshold))
	require.Equal(t, 1, calls, "second read should hit the cache")

	ps.InvalidateSettingsCache()
	require.Equal(t, 7, ps.getSettingInt(context.Background(), settingApprovalThreshold))
	require.Equal(t, 2, calls, "invalidation must force a fresh read")
}

// ---- createReviewSubmission ----

func TestCreateReviewSubmissionAttemptNumbering(t *testing.T) {
	ps := NewPromptService(nil, nil, nil, nil, nil, nil, nil, nil)
	mockReview := new(MockReviewRepository)
	ps.SetReviewRepo(mockReview)

	var created []*models.ReviewSubmission
	mockReview.CreateSubmissionFn = func(ctx context.Context, s *models.ReviewSubmission) error {
		created = append(created, s)
		return nil
	}
	// No prior submission: attempt 1.
	mockReview.GetLatestSubmissionFn = func(ctx context.Context, templateID string) (*models.ReviewSubmission, error) {
		return nil, repository.ErrSubmissionNotFound
	}
	require.NoError(t, ps.createReviewSubmission(context.Background(), "tpl_1", "user_1"))
	require.Len(t, created, 1)
	require.EqualValues(t, 1, created[0].AttemptNumber)
	require.Equal(t, "pending", created[0].Status)

	// Prior rejected submission: attempt 2.
	mockReview.GetLatestSubmissionFn = func(ctx context.Context, templateID string) (*models.ReviewSubmission, error) {
		return &models.ReviewSubmission{AttemptNumber: 1, Status: "rejected"}, nil
	}
	require.NoError(t, ps.createReviewSubmission(context.Background(), "tpl_1", "user_1"))
	require.Len(t, created, 2)
	require.EqualValues(t, 2, created[1].AttemptNumber)
}

func TestCreateReviewSubmissionFailsWhenInsertFails(t *testing.T) {
	ps := NewPromptService(nil, nil, nil, nil, nil, nil, nil, nil)
	mockReview := new(MockReviewRepository)
	ps.SetReviewRepo(mockReview)
	mockReview.GetLatestSubmissionFn = func(ctx context.Context, templateID string) (*models.ReviewSubmission, error) {
		return nil, repository.ErrSubmissionNotFound
	}
	mockReview.CreateSubmissionFn = func(ctx context.Context, s *models.ReviewSubmission) error {
		return errors.New("insert failed")
	}
	err := ps.createReviewSubmission(context.Background(), "tpl_1", "user_1")
	require.Error(t, err)
	require.Equal(t, codes.Internal, status.Code(err))
}

// ---- ListReviewQueue ----

func TestListReviewQueue(t *testing.T) {
	svc, mockUser, _, mockReview := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	expectAdminAuth(mockUser, actor)

	mockReview.ListQueueFn = func(ctx context.Context, f repository.ReviewQueueFilters) ([]*models.ReviewQueueRow, int64, error) {
		require.Equal(t, 20, f.Limit)
		require.Equal(t, 0, f.Offset)
		require.Equal(t, 48, f.AgingHours)
		return []*models.ReviewQueueRow{
			{
				ReviewSubmission: *makePendingSubmission("sub_1", "tpl_1", "user_1", 2),
				TemplateTitle:    "Title",
				TemplateCategory: "writing",
				CreatorName:      "Creator",
				CreatorApproved:  0,
				PriorRejections:  1,
				Aging:            true,
			},
		}, 1, nil
	}

	resp, err := svc.ListReviewQueue(adminCtx(actor.ID), &pb.ListReviewQueueRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	item := resp.Items[0]
	require.Equal(t, "sub_1", item.Id)
	require.True(t, item.IsResubmission)
	require.True(t, item.Aging)
	require.EqualValues(t, 2, item.AttemptNumber)
	require.EqualValues(t, 1, item.CreatorPriorRejections)
}

func TestListReviewQueuePagination(t *testing.T) {
	svc, mockUser, _, mockReview := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	expectAdminAuth(mockUser, actor)

	var captured repository.ReviewQueueFilters
	mockReview.ListQueueFn = func(ctx context.Context, f repository.ReviewQueueFilters) ([]*models.ReviewQueueRow, int64, error) {
		captured = f
		return nil, 0, nil
	}

	// page=2, per_page=50 → offset 50.
	_, err := svc.ListReviewQueue(adminCtx(actor.ID), &pb.ListReviewQueueRequest{Page: 2, PerPage: 50})
	require.NoError(t, err)
	require.Equal(t, 50, captured.Limit)
	require.Equal(t, 50, captured.Offset)

	// per_page capped at 100.
	_, err = svc.ListReviewQueue(adminCtx(actor.ID), &pb.ListReviewQueueRequest{Page: 1, PerPage: 500})
	require.NoError(t, err)
	require.Equal(t, 100, captured.Limit)
}

func TestListReviewQueueRequiresAdmin(t *testing.T) {
	svc, _, _, _ := newReviewTestService(t)
	// No user in context → Unauthenticated.
	_, err := svc.ListReviewQueue(context.Background(), &pb.ListReviewQueueRequest{})
	require.Error(t, err)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	// Authenticated non-admin → PermissionDenied.
	nonAdmin := ContextWithUserID(context.Background(), "user_1")
	_, err = svc.ListReviewQueue(nonAdmin, &pb.ListReviewQueueRequest{})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

// ---- GetReviewSubmission ----

func TestGetReviewSubmission(t *testing.T) {
	svc, mockUser, _, mockReview := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	expectAdminAuth(mockUser, actor)

	mockReview.GetSubmissionDetailFn = func(ctx context.Context, id string) (*models.ReviewSubmissionDetail, error) {
		require.Equal(t, "sub_1", id)
		return &models.ReviewSubmissionDetail{
			ReviewSubmission: *makePendingSubmission("sub_1", "tpl_1", "user_1", 1),
			TemplateTitle:    "Title",
			VersionContent:   "content",
			CreatorName:      "Creator",
			CreatorApproved:  2,
			History: []*models.ReviewSubmission{
				{
					AttemptNumber:    1,
					Status:           "rejected",
					RejectReasonCode: nullString("spam"),
					RejectNote:       nullString("duplicate content"),
					DecidedAt:        sql.NullTime{Time: time.Now(), Valid: true},
				},
			},
		}, nil
	}

	resp, err := svc.GetReviewSubmission(adminCtx(actor.ID), &pb.GetReviewSubmissionRequest{Id: "sub_1"})
	require.NoError(t, err)
	d := resp.Submission
	require.Equal(t, "tpl_1", d.TemplateId)
	require.Equal(t, "content", d.TemplateContent)
	require.EqualValues(t, 2, d.CreatorPublishedCount)
	require.Len(t, d.Chain, 1)
	require.Equal(t, "spam", d.Chain[0].RejectReasonCode)
}

func TestGetReviewSubmissionNotFound(t *testing.T) {
	svc, mockUser, _, _ := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	expectAdminAuth(mockUser, actor)

	_, err := svc.GetReviewSubmission(adminCtx(actor.ID), &pb.GetReviewSubmissionRequest{Id: "missing"})
	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))
}

// ---- ApproveReviewSubmission ----

func TestApproveReviewSubmission(t *testing.T) {
	svc, mockUser, mockAdmin, mockReview := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	expectAdminAuth(mockUser, actor)

	approved := false
	mockReview.DecideSubmissionFn = func(ctx context.Context, id, decidedStatus, decidedBy string, reasonCode, note *string) (*models.ReviewSubmission, error) {
		require.Equal(t, "approved", decidedStatus)
		require.Equal(t, actor.ID, decidedBy)
		approved = true
		return makePendingSubmission(id, "tpl_1", "user_1", 1), nil
	}
	mockAdmin.On("SetTemplateModeration", mock.Anything, "tpl_1", "normal", true).Return(nil).Once()
	mockAdmin.On("InsertAuditLog", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReview.IncrementApprovedFn = func(ctx context.Context, userID string) error {
		require.Equal(t, "user_1", userID)
		return nil
	}

	resp, err := svc.ApproveReviewSubmission(adminCtx(actor.ID), &pb.ApproveReviewSubmissionRequest{Id: "sub_1"})
	require.NoError(t, err)
	require.True(t, approved)
	require.Equal(t, "approved", resp.Status)
	require.Equal(t, "tpl_1", resp.TemplateId)
	mockAdmin.AssertExpectations(t)
}

func TestApproveReviewSubmissionAlreadyDecided(t *testing.T) {
	svc, mockUser, _, mockReview := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	expectAdminAuth(mockUser, actor)

	mockReview.DecideSubmissionFn = func(ctx context.Context, id, decidedStatus, decidedBy string, reasonCode, note *string) (*models.ReviewSubmission, error) {
		return nil, repository.ErrAlreadyDecided
	}

	_, err := svc.ApproveReviewSubmission(adminCtx(actor.ID), &pb.ApproveReviewSubmissionRequest{Id: "sub_1"})
	require.Error(t, err)
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
}

// ---- RejectReviewSubmission ----

func TestRejectReviewSubmission(t *testing.T) {
	svc, mockUser, mockAdmin, mockReview := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	expectAdminAuth(mockUser, actor)

	var gotReason, gotNote *string
	mockReview.DecideSubmissionFn = func(ctx context.Context, id, decidedStatus, decidedBy string, reasonCode, note *string) (*models.ReviewSubmission, error) {
		require.Equal(t, "rejected", decidedStatus)
		gotReason, gotNote = reasonCode, note
		return makePendingSubmission(id, "tpl_1", "user_1", 1), nil
	}
	mockAdmin.On("SetTemplateModeration", mock.Anything, "tpl_1", "rejected", false).Return(nil).Once()
	mockAdmin.On("InsertAuditLog", mock.Anything, mock.Anything).Return(nil).Maybe()

	resp, err := svc.RejectReviewSubmission(adminCtx(actor.ID), &pb.RejectReviewSubmissionRequest{
		Id:         "sub_1",
		ReasonCode: "spam",
		Note:       "duplicate content",
	})
	require.NoError(t, err)
	require.Equal(t, "rejected", resp.Status)
	require.NotNil(t, gotReason)
	require.Equal(t, "spam", *gotReason)
	require.NotNil(t, gotNote)
	require.Equal(t, "duplicate content", *gotNote)
	mockAdmin.AssertExpectations(t)
}

func TestRejectReviewSubmissionValidation(t *testing.T) {
	svc, mockUser, _, _ := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	expectAdminAuth(mockUser, actor)

	tests := []struct {
		name   string
		reason string
		note   string
	}{
		{name: "invalid reason code", reason: "badReason"},
		{name: "empty reason code", reason: ""},
		{name: "note too long", reason: "spam", note: string(make([]byte, 501))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.RejectReviewSubmission(adminCtx(actor.ID), &pb.RejectReviewSubmissionRequest{
				Id:         "sub_1",
				ReasonCode: tt.reason,
				Note:       tt.note,
			})
			require.Error(t, err)
			require.Equal(t, codes.InvalidArgument, status.Code(err))
		})
	}
}

func TestRejectReviewSubmissionNotFound(t *testing.T) {
	svc, mockUser, _, mockReview := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	expectAdminAuth(mockUser, actor)

	mockReview.DecideSubmissionFn = func(ctx context.Context, id, decidedStatus, decidedBy string, reasonCode, note *string) (*models.ReviewSubmission, error) {
		return nil, repository.ErrSubmissionNotFound
	}

	_, err := svc.RejectReviewSubmission(adminCtx(actor.ID), &pb.RejectReviewSubmissionRequest{Id: "sub_1", ReasonCode: "spam"})
	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))
}

// ---- GetReviewMetrics ----

func TestGetReviewMetrics(t *testing.T) {
	svc, mockUser, _, mockReview := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	expectAdminAuth(mockUser, actor)

	mockReview.GetMetricsFn = func(ctx context.Context) (*models.ReviewMetrics, error) {
		return &models.ReviewMetrics{
			PendingCount:  4,
			MedianHours:   12.5,
			ApprovedToday: 2,
			RejectedToday: 1,
		}, nil
	}

	resp, err := svc.GetReviewMetrics(adminCtx(actor.ID), &pb.GetReviewMetricsRequest{})
	require.NoError(t, err)
	require.EqualValues(t, 4, resp.OpenCount)
	require.Equal(t, 12.5, resp.MedianTimeInReviewHours)
	require.EqualValues(t, 3, resp.DecisionsToday)
}

// ---- Platform settings ----

func TestGetPlatformSettings(t *testing.T) {
	svc, mockUser, _, mockReview := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	expectAdminAuth(mockUser, actor)

	mockReview.GetSettingsFn = func(ctx context.Context) ([]*models.PlatformSetting, error) {
		return []*models.PlatformSetting{
			{Key: settingApprovalThreshold, Value: "3"},
			{Key: settingRegateCount, Value: "3"},
			{Key: settingAgingHours, Value: "48"},
		}, nil
	}

	resp, err := svc.GetPlatformSettings(adminCtx(actor.ID), &pb.GetPlatformSettingsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Settings, 3)
}

func TestUpdatePlatformSetting(t *testing.T) {
	svc, mockUser, mockAdmin, mockReview := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	expectAdminAuth(mockUser, actor)

	upserted := ""
	mockReview.UpsertSettingFn = func(ctx context.Context, key, value string) error {
		upserted = key + "=" + value
		return nil
	}
	mockAdmin.On("InsertAuditLog", mock.Anything, mock.Anything).Return(nil).Maybe()

	// Wire a prompt service so cache invalidation is exercised too.
	ps := NewPromptService(nil, nil, nil, nil, nil, nil, nil, nil)
	psMock := new(MockReviewRepository)
	psMock.GetSettingFn = func(ctx context.Context, key string) (string, error) {
		return "3", nil
	}
	ps.SetReviewRepo(psMock)
	svc.SetPromptService(ps)
	require.Equal(t, 3, ps.getSettingInt(context.Background(), settingApprovalThreshold))

	resp, err := svc.UpdatePlatformSetting(adminCtx(actor.ID), &pb.UpdatePlatformSettingRequest{
		Key:    settingApprovalThreshold,
		Value:  "5",
		Reason: "raise the bar",
	})
	require.NoError(t, err)
	require.Equal(t, "5", resp.Value)
	require.Equal(t, settingApprovalThreshold+"=5", upserted)

	// Cache was invalidated: the next read must go back to the repo.
	psMock.GetSettingFn = func(ctx context.Context, key string) (string, error) {
		return "5", nil
	}
	require.Equal(t, 5, ps.getSettingInt(context.Background(), settingApprovalThreshold))
}

func TestUpdatePlatformSettingValidation(t *testing.T) {
	svc, mockUser, _, _ := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	expectAdminAuth(mockUser, actor)

	tests := []struct {
		name   string
		key    string
		value  string
		reason string
	}{
		{name: "unknown key", key: "unknown_key", value: "5", reason: "valid reason"},
		{name: "non-numeric value", key: settingApprovalThreshold, value: "abc", reason: "valid reason"},
		{name: "zero regate count", key: settingRegateCount, value: "0", reason: "valid reason"},
		{name: "zero aging hours", key: settingAgingHours, value: "0", reason: "valid reason"},
		{name: "negative value", key: settingApprovalThreshold, value: "-1", reason: "valid reason"},
		{name: "short reason", key: settingApprovalThreshold, value: "5", reason: "no"},
		{name: "empty reason", key: settingApprovalThreshold, value: "5", reason: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.UpdatePlatformSetting(adminCtx(actor.ID), &pb.UpdatePlatformSettingRequest{
				Key: tt.key, Value: tt.value, Reason: tt.reason,
			})
			require.Error(t, err)
			require.Equal(t, codes.InvalidArgument, status.Code(err))
		})
	}

	// Threshold 0 is the explicit "gating disabled" switch and must be valid.
	t.Run("zero threshold disables gating", func(t *testing.T) {
		svc2, mockUser2, mockAdmin2, mockReview2 := newReviewTestService(t)
		actor2 := makeAdminUser("admin_2")
		expectAdminAuth(mockUser2, actor2)
		var upserted string
		mockReview2.UpsertSettingFn = func(ctx context.Context, key, value string) error {
			upserted = key + "=" + value
			return nil
		}
		mockAdmin2.On("InsertAuditLog", mock.Anything, mock.Anything).Return(nil).Maybe()
		resp, err := svc2.UpdatePlatformSetting(adminCtx(actor2.ID), &pb.UpdatePlatformSettingRequest{
			Key: settingApprovalThreshold, Value: "0", Reason: "disable gating",
		})
		require.NoError(t, err)
		require.Equal(t, "0", resp.Value)
		require.Equal(t, settingApprovalThreshold+"=0", upserted)
	})
}

// ---- BanUser re-gate hook ----

func TestBanUserSetsGatedRemaining(t *testing.T) {
	svc, mockUser, mockAdmin, mockRedis, _ := newAdminTestService(t)
	mockReview := new(MockReviewRepository)
	svc.SetReviewRepo(mockReview)

	target := makeRegularUser("user_1")
	mockUser.On("GetByID", mock.Anything, "admin_1").Return(makeAdminUser("admin_1"), nil)
	mockAdmin.On("GetUserDetail", mock.Anything, target.ID).Return(target, nil)
	mockUser.On("UpdateRoleAndStatus", mock.Anything, target.ID, "user", "banned").Return(nil)
	mockAdmin.On("InsertAuditLog", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockRedis.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	setCalled := false
	mockReview.SetGatedRemainingFn = func(ctx context.Context, userID string, value int32) error {
		setCalled = true
		require.Equal(t, target.ID, userID)
		require.EqualValues(t, 3, value) // default regate count
		return nil
	}

	_, err := svc.BanUser(adminCtx("admin_1"), &pb.BanUserRequest{UserId: target.ID, Reason: "spamming the platform"})
	require.NoError(t, err)
	require.True(t, setCalled, "BanUser must re-gate the user's next publishes")
}

// ---- Decision notifications ----

func TestNotifyReviewDecision(t *testing.T) {
	svc, _, _, _ := newReviewTestService(t)
	notifRepo := new(MockNotificationRepository)
	svc.SetNotificationRepo(notifRepo)
	actor := makeAdminUser("admin_1")
	actor.DisplayName = "Moderator"

	// Approve: actor display name is carried.
	notifRepo.On("Create", mock.Anything, mock.MatchedBy(func(n *models.Notification) bool {
		return n.RecipientID == "user_1" && n.ActorID == "admin_1" &&
			n.Type == "template_approved" && n.ActorDisplayName == "Moderator"
	})).Return(nil).Once()
	svc.notifyReviewDecision(context.Background(), actor, "user_1", "tpl_1", "template_approved", "")
	notifRepo.AssertExpectations(t)

	// Reject: the reason code rides in the display-name slot.
	notifRepo.On("Create", mock.Anything, mock.MatchedBy(func(n *models.Notification) bool {
		return n.Type == "template_rejected" && n.ActorDisplayName == "spam"
	})).Return(nil).Once()
	svc.notifyReviewDecision(context.Background(), actor, "user_1", "tpl_1", "template_rejected", "spam")
	notifRepo.AssertExpectations(t)
}

func TestNotifyReviewDecisionSelfSuppressed(t *testing.T) {
	svc, _, _, _ := newReviewTestService(t)
	notifRepo := new(MockNotificationRepository)
	svc.SetNotificationRepo(notifRepo)

	// An admin deciding their own submission: no notification.
	svc.notifyReviewDecision(context.Background(), makeAdminUser("admin_1"), "admin_1", "tpl_1", "template_approved", "")
	// Nil repo and nil actor are no-ops too.
	svc.SetNotificationRepo(nil)
	svc.notifyReviewDecision(context.Background(), makeAdminUser("admin_1"), "user_1", "tpl_1", "template_approved", "")
	notifRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestNotifyReviewDecisionFailureNonFatal(t *testing.T) {
	svc, _, _, _ := newReviewTestService(t)
	notifRepo := new(MockNotificationRepository)
	svc.SetNotificationRepo(notifRepo)
	notifRepo.On("Create", mock.Anything, mock.Anything).Return(errors.New("insert failed")).Once()

	// A notification failure must not panic or propagate.
	svc.notifyReviewDecision(context.Background(), makeAdminUser("admin_1"), "user_1", "tpl_1", "template_approved", "")
	notifRepo.AssertExpectations(t)
}

// ---- Gating hooks in CreateTemplate / UpdateTemplate ----

// newGatingTestService builds a PromptService with func-based mocks wired
// for the gating paths.
func newGatingTestService() (*PromptService, *MockTemplateRepository, *MockTemplateVersionRepository, *MockReviewRepository) {
	tplRepo := new(MockTemplateRepository)
	verRepo := new(MockTemplateVersionRepository)
	aliasRepo := new(MockTemplateAliasRepository)
	reviewRepo := new(MockReviewRepository)
	ps := NewPromptService(nil, tplRepo, verRepo, aliasRepo, nil, nil, nil, nil)
	ps.SetReviewRepo(reviewRepo)
	return ps, tplRepo, verRepo, reviewRepo
}

func TestCreateTemplateGating(t *testing.T) {
	t.Run("gated creator public create enters review", func(t *testing.T) {
		ps, tplRepo, _, reviewRepo := newGatingTestService()
		reviewRepo.GetUserGatingStateFn = func(ctx context.Context, userID string) (int32, int32, error) {
			return 0, 0, nil // 0 approved < threshold 3
		}
		var created *models.Template
		tplRepo.CreateFunc = func(ctx context.Context, tpl *models.Template) error {
			tpl.ID = "tpl_1"
			created = tpl
			return nil
		}
		reviewRepo.CreateSubmissionFn = func(ctx context.Context, s *models.ReviewSubmission) error {
			s.ID = "sub_1"
			return nil
		}
		reviewRepo.GetLatestSubmissionFn = func(ctx context.Context, templateID string) (*models.ReviewSubmission, error) {
			return nil, repository.ErrSubmissionNotFound
		}

		resp, err := ps.CreateTemplate(ContextWithUserID(context.Background(), "user_1"), &pb.CreateTemplateRequest{
			Title:      "T",
			Content:    "C",
			Visibility: pb.Visibility_VISIBILITY_PUBLIC,
		})
		require.NoError(t, err)
		require.Equal(t, "in_review", resp.Template.ModerationStatus)
		require.Nil(t, resp.Template.PublishedAt)
		require.Equal(t, "in_review", created.ModerationStatus)
		require.False(t, created.PublishedAt.Valid)
	})

	t.Run("exempt creator public create publishes immediately", func(t *testing.T) {
		ps, tplRepo, _, reviewRepo := newGatingTestService()
		reviewRepo.GetUserGatingStateFn = func(ctx context.Context, userID string) (int32, int32, error) {
			return 3, 0, nil // at threshold
		}
		var created *models.Template
		tplRepo.CreateFunc = func(ctx context.Context, tpl *models.Template) error {
			tpl.ID = "tpl_2"
			created = tpl
			return nil
		}

		resp, err := ps.CreateTemplate(ContextWithUserID(context.Background(), "user_1"), &pb.CreateTemplateRequest{
			Title:      "T",
			Content:    "C",
			Visibility: pb.Visibility_VISIBILITY_PUBLIC,
		})
		require.NoError(t, err)
		require.Equal(t, "normal", resp.Template.ModerationStatus)
		require.NotNil(t, resp.Template.PublishedAt)
		require.True(t, created.PublishedAt.Valid)
	})

	t.Run("private create is never gated", func(t *testing.T) {
		ps, tplRepo, _, reviewRepo := newGatingTestService()
		gatingCalled := false
		reviewRepo.GetUserGatingStateFn = func(ctx context.Context, userID string) (int32, int32, error) {
			gatingCalled = true
			return 0, 0, nil
		}
		tplRepo.CreateFunc = func(ctx context.Context, tpl *models.Template) error {
			tpl.ID = "tpl_3"
			return nil
		}

		resp, err := ps.CreateTemplate(ContextWithUserID(context.Background(), "user_1"), &pb.CreateTemplateRequest{
			Title:      "T",
			Content:    "C",
			Visibility: pb.Visibility_VISIBILITY_PRIVATE,
		})
		require.NoError(t, err)
		require.Equal(t, "normal", resp.Template.ModerationStatus)
		require.False(t, gatingCalled, "private creates must not consult gating state")
	})

	t.Run("submission insert failure fails the publish", func(t *testing.T) {
		ps, tplRepo, _, reviewRepo := newGatingTestService()
		reviewRepo.GetUserGatingStateFn = func(ctx context.Context, userID string) (int32, int32, error) {
			return 0, 0, nil
		}
		tplRepo.CreateFunc = func(ctx context.Context, tpl *models.Template) error {
			tpl.ID = "tpl_4"
			return nil
		}
		reviewRepo.GetLatestSubmissionFn = func(ctx context.Context, templateID string) (*models.ReviewSubmission, error) {
			return nil, repository.ErrSubmissionNotFound
		}
		reviewRepo.CreateSubmissionFn = func(ctx context.Context, s *models.ReviewSubmission) error {
			return errors.New("queue write failed")
		}

		_, err := ps.CreateTemplate(ContextWithUserID(context.Background(), "user_1"), &pb.CreateTemplateRequest{
			Title:      "T",
			Content:    "C",
			Visibility: pb.Visibility_VISIBILITY_PUBLIC,
		})
		require.Error(t, err)
		require.Equal(t, codes.Internal, status.Code(err))
	})
}

func TestUpdateTemplateGatingTransition(t *testing.T) {
	baseTemplate := func(visibility, moderation string) *models.Template {
		return &models.Template{
			ID:               "tpl_1",
			OwnerID:          "user_1",
			Title:            "T",
			Visibility:       visibility,
			ModerationStatus: moderation,
		}
	}

	t.Run("private to public by gated creator enters review", func(t *testing.T) {
		ps, tplRepo, _, reviewRepo := newGatingTestService()
		reviewRepo.GetUserGatingStateFn = func(ctx context.Context, userID string) (int32, int32, error) {
			return 0, 0, nil
		}
		tpl := baseTemplate("private", "normal")
		tplRepo.GetFunc = func(ctx context.Context, id, currentUserID string) (*models.Template, error) {
			return tpl, nil
		}
		var updated *models.Template
		tplRepo.UpdateFunc = func(ctx context.Context, t *models.Template) error {
			updated = t
			return nil
		}
		reviewRepo.GetLatestSubmissionFn = func(ctx context.Context, templateID string) (*models.ReviewSubmission, error) {
			return nil, repository.ErrSubmissionNotFound
		}
		submissionCreated := false
		reviewRepo.CreateSubmissionFn = func(ctx context.Context, s *models.ReviewSubmission) error {
			submissionCreated = true
			require.EqualValues(t, 1, s.AttemptNumber)
			return nil
		}

		resp, err := ps.UpdateTemplate(ContextWithUserID(context.Background(), "user_1"), &pb.UpdateTemplateRequest{
			TemplateId: "tpl_1",
			Content:    "new content",
			Visibility: pb.Visibility_VISIBILITY_PUBLIC,
		})
		require.NoError(t, err)
		require.Equal(t, "in_review", resp.Template.ModerationStatus)
		require.True(t, submissionCreated)
		require.Equal(t, "in_review", updated.ModerationStatus)
	})

	t.Run("rejected to public resubmission chains attempts", func(t *testing.T) {
		ps, tplRepo, _, reviewRepo := newGatingTestService()
		reviewRepo.GetUserGatingStateFn = func(ctx context.Context, userID string) (int32, int32, error) {
			return 0, 0, nil
		}
		// A rejected template is still public-visibility in the DB; the
		// moderation status marks it rejected. Re-publishing transitions it.
		tpl := baseTemplate("private", "rejected")
		tplRepo.GetFunc = func(ctx context.Context, id, currentUserID string) (*models.Template, error) {
			return tpl, nil
		}
		tplRepo.UpdateFunc = func(ctx context.Context, t *models.Template) error { return nil }
		reviewRepo.GetLatestSubmissionFn = func(ctx context.Context, templateID string) (*models.ReviewSubmission, error) {
			return &models.ReviewSubmission{AttemptNumber: 1, Status: "rejected"}, nil
		}
		var attempt int32
		reviewRepo.CreateSubmissionFn = func(ctx context.Context, s *models.ReviewSubmission) error {
			attempt = s.AttemptNumber
			return nil
		}

		_, err := ps.UpdateTemplate(ContextWithUserID(context.Background(), "user_1"), &pb.UpdateTemplateRequest{
			TemplateId: "tpl_1",
			Content:    "fixed content",
			Visibility: pb.Visibility_VISIBILITY_PUBLIC,
		})
		require.NoError(t, err)
		require.EqualValues(t, 2, attempt, "resubmission must chain attempt numbers")
	})

	t.Run("edits without publish transition keep moderation status", func(t *testing.T) {
		ps, tplRepo, _, reviewRepo := newGatingTestService()
		tpl := baseTemplate("public", "in_review")
		tplRepo.GetFunc = func(ctx context.Context, id, currentUserID string) (*models.Template, error) {
			return tpl, nil
		}
		var updated *models.Template
		tplRepo.UpdateFunc = func(ctx context.Context, t *models.Template) error {
			updated = t
			return nil
		}
		gatingCalled := false
		reviewRepo.GetUserGatingStateFn = func(ctx context.Context, userID string) (int32, int32, error) {
			gatingCalled = true
			return 0, 0, nil
		}

		_, err := ps.UpdateTemplate(ContextWithUserID(context.Background(), "user_1"), &pb.UpdateTemplateRequest{
			TemplateId: "tpl_1",
			Content:    "tweak",
		})
		require.NoError(t, err)
		require.Equal(t, "in_review", updated.ModerationStatus, "in_review edits must stay in_review")
		require.False(t, gatingCalled, "non-transition edits must not consult gating")
	})

	t.Run("exempt creator private to public publishes immediately", func(t *testing.T) {
		ps, tplRepo, _, reviewRepo := newGatingTestService()
		reviewRepo.GetUserGatingStateFn = func(ctx context.Context, userID string) (int32, int32, error) {
			return 5, 0, nil
		}
		tpl := baseTemplate("private", "normal")
		tplRepo.GetFunc = func(ctx context.Context, id, currentUserID string) (*models.Template, error) {
			return tpl, nil
		}
		var updated *models.Template
		tplRepo.UpdateFunc = func(ctx context.Context, t *models.Template) error {
			updated = t
			return nil
		}

		resp, err := ps.UpdateTemplate(ContextWithUserID(context.Background(), "user_1"), &pb.UpdateTemplateRequest{
			TemplateId: "tpl_1",
			Content:    "c",
			Visibility: pb.Visibility_VISIBILITY_PUBLIC,
		})
		require.NoError(t, err)
		require.Equal(t, "normal", resp.Template.ModerationStatus)
		require.True(t, updated.PublishedAt.Valid)
	})

	t.Run("rejected public template re-publish re-enters review", func(t *testing.T) {
		ps, tplRepo, _, reviewRepo := newGatingTestService()
		reviewRepo.GetUserGatingStateFn = func(ctx context.Context, userID string) (int32, int32, error) {
			return 0, 0, nil
		}
		// A rejected template keeps visibility=public; the creator edits and
		// re-publishes (FR-6 resubmission) — the transition is detected via
		// moderation_status, not visibility.
		tpl := baseTemplate("public", "rejected")
		tplRepo.GetFunc = func(ctx context.Context, id, currentUserID string) (*models.Template, error) {
			require.Equal(t, "user_1", currentUserID, "update must fetch with the caller id so owners can edit gated templates")
			return tpl, nil
		}
		var updated *models.Template
		tplRepo.UpdateFunc = func(ctx context.Context, t *models.Template) error {
			updated = t
			return nil
		}
		reviewRepo.GetLatestSubmissionFn = func(ctx context.Context, templateID string) (*models.ReviewSubmission, error) {
			return &models.ReviewSubmission{AttemptNumber: 1, Status: "rejected"}, nil
		}
		var attempt int32
		reviewRepo.CreateSubmissionFn = func(ctx context.Context, s *models.ReviewSubmission) error {
			attempt = s.AttemptNumber
			return nil
		}

		resp, err := ps.UpdateTemplate(ContextWithUserID(context.Background(), "user_1"), &pb.UpdateTemplateRequest{
			TemplateId: "tpl_1",
			Content:    "fixed content",
			Visibility: pb.Visibility_VISIBILITY_PUBLIC,
		})
		require.NoError(t, err)
		require.Equal(t, "in_review", resp.Template.ModerationStatus)
		require.EqualValues(t, 2, attempt, "resubmission must chain attempt numbers")
		require.Equal(t, "in_review", updated.ModerationStatus)
		require.False(t, updated.PublishedAt.Valid, "in_review templates must clear published_at")
	})

	t.Run("exempt creator rejected re-publish goes live", func(t *testing.T) {
		ps, tplRepo, _, reviewRepo := newGatingTestService()
		reviewRepo.GetUserGatingStateFn = func(ctx context.Context, userID string) (int32, int32, error) {
			return 5, 0, nil
		}
		tpl := baseTemplate("public", "rejected")
		tplRepo.GetFunc = func(ctx context.Context, id, currentUserID string) (*models.Template, error) {
			return tpl, nil
		}
		var updated *models.Template
		tplRepo.UpdateFunc = func(ctx context.Context, t *models.Template) error {
			updated = t
			return nil
		}

		resp, err := ps.UpdateTemplate(ContextWithUserID(context.Background(), "user_1"), &pb.UpdateTemplateRequest{
			TemplateId: "tpl_1",
			Content:    "c",
			Visibility: pb.Visibility_VISIBILITY_PUBLIC,
		})
		require.NoError(t, err)
		require.Equal(t, "normal", resp.Template.ModerationStatus)
		require.True(t, updated.PublishedAt.Valid)
	})
}

func TestHiddenTemplateReadGuards(t *testing.T) {
	hiddenTpl := &models.Template{
		ID:               "tpl_1",
		OwnerID:          "owner_1",
		Visibility:       "public",
		ModerationStatus: "in_review",
	}

	t.Run("list versions hidden for non-owner", func(t *testing.T) {
		ps, tplRepo, _, _ := newGatingTestService()
		tplRepo.GetFunc = func(ctx context.Context, id, currentUserID string) (*models.Template, error) {
			if currentUserID != "owner_1" {
				return nil, errors.New("template not found")
			}
			return hiddenTpl, nil
		}
		_, err := ps.ListTemplateVersions(ContextWithUserID(context.Background(), "user_2"), &pb.ListTemplateVersionsRequest{
			TemplateId: "tpl_1",
		})
		require.Error(t, err)
		require.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("list versions visible for owner", func(t *testing.T) {
		ps, tplRepo, verRepo, _ := newGatingTestService()
		tplRepo.GetFunc = func(ctx context.Context, id, currentUserID string) (*models.Template, error) {
			if currentUserID != "owner_1" {
				return nil, errors.New("template not found")
			}
			return hiddenTpl, nil
		}
		verRepo.On("List", mock.Anything, 10, 0, "tpl_1").Return([]*models.TemplateVersion{
			{ID: 1, Version: 1, Content: "c"},
		}, nil).Once()
		resp, err := ps.ListTemplateVersions(ContextWithUserID(context.Background(), "owner_1"), &pb.ListTemplateVersionsRequest{
			TemplateId: "tpl_1",
		})
		require.NoError(t, err)
		require.Len(t, resp.Versions, 1)
	})

	t.Run("list prompts by hidden template 404s for non-owner", func(t *testing.T) {
		ps, tplRepo, _, _ := newGatingTestService()
		tplRepo.GetFunc = func(ctx context.Context, id, currentUserID string) (*models.Template, error) {
			if currentUserID != "owner_1" {
				return nil, errors.New("template not found")
			}
			return hiddenTpl, nil
		}
		_, err := ps.ListPrompts(ContextWithUserID(context.Background(), "user_2"), &pb.ListPromptsRequest{
			TemplateId: "tpl_1",
		})
		require.Error(t, err)
		require.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("get prompt by alias hidden for non-owner", func(t *testing.T) {
		ps, tplRepo, _, _ := newGatingTestService()
		tplRepo.GetFunc = func(ctx context.Context, id, currentUserID string) (*models.Template, error) {
			if currentUserID != "owner_1" {
				return nil, errors.New("template not found")
			}
			return hiddenTpl, nil
		}
		aliasRepo := &MockTemplateAliasRepository{
			GetFunc: func(ctx context.Context, templateID, aliasName string) (*models.TemplateAlias, error) {
				return &models.TemplateAlias{TemplateID: "tpl_1", AliasName: "latest", VersionID: 7}, nil
			},
		}
		ps.TemplateAliasRepo = aliasRepo
		ps.TemplateVersionRepo = nil // not reached; guard fires first
		_, err := ps.GetPromptByAlias(ContextWithUserID(context.Background(), "user_2"), &pb.GetPromptByAliasRequest{
			TemplateId: "tpl_1",
			AliasName:  "latest",
		})
		require.Error(t, err)
		require.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("alias mutation hidden for non-owner", func(t *testing.T) {
		ps, tplRepo, _, _ := newGatingTestService()
		tplRepo.GetFunc = func(ctx context.Context, id, currentUserID string) (*models.Template, error) {
			if currentUserID != "owner_1" {
				return nil, errors.New("template not found")
			}
			return hiddenTpl, nil
		}
		err := ps.authorizeAliasMutation(ContextWithUserID(context.Background(), "user_2"), "tpl_1")
		require.Error(t, err)
		require.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("alias mutation allowed for owner", func(t *testing.T) {
		ps, tplRepo, _, _ := newGatingTestService()
		tplRepo.GetFunc = func(ctx context.Context, id, currentUserID string) (*models.Template, error) {
			if currentUserID != "owner_1" {
				return nil, errors.New("template not found")
			}
			return hiddenTpl, nil
		}
		err := ps.authorizeAliasMutation(ContextWithUserID(context.Background(), "owner_1"), "tpl_1")
		require.NoError(t, err)
	})

	t.Run("delete hidden template 404s for non-owner but works for owner", func(t *testing.T) {
		ps, tplRepo, _, _ := newGatingTestService()
		tplRepo.GetFunc = func(ctx context.Context, id, currentUserID string) (*models.Template, error) {
			if currentUserID != "owner_1" {
				return nil, errors.New("template not found")
			}
			return hiddenTpl, nil
		}
		deleted := false
		tplRepo.DeleteFunc = func(ctx context.Context, id string) error {
			deleted = true
			return nil
		}

		_, err := ps.DeleteTemplate(ContextWithUserID(context.Background(), "user_2"), &pb.DeleteTemplateRequest{Id: "tpl_1"})
		require.Error(t, err)
		require.Equal(t, codes.NotFound, status.Code(err))
		require.False(t, deleted)

		resp, err := ps.DeleteTemplate(ContextWithUserID(context.Background(), "owner_1"), &pb.DeleteTemplateRequest{Id: "tpl_1"})
		require.NoError(t, err)
		require.True(t, resp.Success)
		require.True(t, deleted)
	})
}

// nullString is a test helper building sql.NullString.
func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: true}
}

// ---- Error paths for coverage ----

func TestGetReviewSubmissionRepoError(t *testing.T) {
	svc, mockUser, _, mockReview := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	expectAdminAuth(mockUser, actor)

	mockReview.GetSubmissionDetailFn = func(ctx context.Context, id string) (*models.ReviewSubmissionDetail, error) {
		return nil, fmt.Errorf("db down")
	}

	_, err := svc.GetReviewSubmission(adminCtx(actor.ID), &pb.GetReviewSubmissionRequest{Id: "sub_1"})
	require.Error(t, err)
	require.Equal(t, codes.Internal, status.Code(err))
}

func TestApproveReviewSubmissionPublishFails(t *testing.T) {
	svc, mockUser, mockAdmin, mockReview := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	expectAdminAuth(mockUser, actor)

	mockReview.DecideSubmissionFn = func(ctx context.Context, id, decidedStatus, decidedBy string, reasonCode, note *string) (*models.ReviewSubmission, error) {
		return makePendingSubmission(id, "tpl_1", "user_1", 1), nil
	}
	mockAdmin.On("SetTemplateModeration", mock.Anything, "tpl_1", "normal", true).Return(errors.New("publish failed")).Once()

	_, err := svc.ApproveReviewSubmission(adminCtx(actor.ID), &pb.ApproveReviewSubmissionRequest{Id: "sub_1"})
	require.Error(t, err)
	require.Equal(t, codes.Internal, status.Code(err))
	mockAdmin.AssertExpectations(t)
}

func TestApproveReviewSubmissionIncrementFailsNonFatal(t *testing.T) {
	svc, mockUser, mockAdmin, mockReview := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	expectAdminAuth(mockUser, actor)

	mockReview.DecideSubmissionFn = func(ctx context.Context, id, decidedStatus, decidedBy string, reasonCode, note *string) (*models.ReviewSubmission, error) {
		return makePendingSubmission(id, "tpl_1", "user_1", 1), nil
	}
	mockAdmin.On("SetTemplateModeration", mock.Anything, "tpl_1", "normal", true).Return(nil).Once()
	mockAdmin.On("InsertAuditLog", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReview.IncrementApprovedFn = func(ctx context.Context, userID string) error {
		return errors.New("counter broken")
	}

	resp, err := svc.ApproveReviewSubmission(adminCtx(actor.ID), &pb.ApproveReviewSubmissionRequest{Id: "sub_1"})
	require.NoError(t, err)
	require.Equal(t, "approved", resp.Status)
	mockAdmin.AssertExpectations(t)
}

func TestRejectReviewSubmissionHideFails(t *testing.T) {
	svc, mockUser, mockAdmin, mockReview := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	expectAdminAuth(mockUser, actor)

	mockReview.DecideSubmissionFn = func(ctx context.Context, id, decidedStatus, decidedBy string, reasonCode, note *string) (*models.ReviewSubmission, error) {
		return makePendingSubmission(id, "tpl_1", "user_1", 1), nil
	}
	mockAdmin.On("SetTemplateModeration", mock.Anything, "tpl_1", "rejected", false).Return(errors.New("hide failed")).Once()

	_, err := svc.RejectReviewSubmission(adminCtx(actor.ID), &pb.RejectReviewSubmissionRequest{Id: "sub_1", ReasonCode: "spam"})
	require.Error(t, err)
	require.Equal(t, codes.Internal, status.Code(err))
	mockAdmin.AssertExpectations(t)
}

func TestGetReviewMetricsRepoError(t *testing.T) {
	svc, mockUser, _, mockReview := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	expectAdminAuth(mockUser, actor)

	mockReview.GetMetricsFn = func(ctx context.Context) (*models.ReviewMetrics, error) {
		return nil, errors.New("metrics unavailable")
	}

	_, err := svc.GetReviewMetrics(adminCtx(actor.ID), &pb.GetReviewMetricsRequest{})
	require.Error(t, err)
	require.Equal(t, codes.Internal, status.Code(err))
}

func TestGetPlatformSettingsRepoError(t *testing.T) {
	svc, mockUser, _, mockReview := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	expectAdminAuth(mockUser, actor)

	mockReview.GetSettingsFn = func(ctx context.Context) ([]*models.PlatformSetting, error) {
		return nil, errors.New("settings unavailable")
	}

	_, err := svc.GetPlatformSettings(adminCtx(actor.ID), &pb.GetPlatformSettingsRequest{})
	require.Error(t, err)
	require.Equal(t, codes.Internal, status.Code(err))
}

func TestNotifyReviewDecisionNoDisplayName(t *testing.T) {
	svc, mockUser, _, _ := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	actor.DisplayName = ""
	expectAdminAuth(mockUser, actor)

	var created *models.Notification
	notifMock := new(MockNotificationRepository)
	notifMock.On("Create", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { created = args.Get(1).(*models.Notification) }).
		Return(nil).Once()
	svc.SetNotificationRepo(notifMock)

	svc.notifyReviewDecision(context.Background(), actor, "user_1", "tpl_1", "template_approved", "")
	require.NotNil(t, created)
	require.Equal(t, "admin_1", created.ActorDisplayName)
	notifMock.AssertExpectations(t)
}

func TestCreateReviewSubmissionHistoryLoadFails(t *testing.T) {
	ps, _, _, reviewRepo := newGatingTestService()
	reviewRepo.GetLatestSubmissionFn = func(ctx context.Context, templateID string) (*models.ReviewSubmission, error) {
		return nil, errors.New("db down")
	}
	err := ps.createReviewSubmission(context.Background(), "tpl_1", "user_1")
	require.Error(t, err)
	require.Equal(t, codes.Internal, status.Code(err))
}

func TestCreateReviewSubmissionDecrementFailsNonFatal(t *testing.T) {
	ps, _, _, reviewRepo := newGatingTestService()
	reviewRepo.GetLatestSubmissionFn = func(ctx context.Context, templateID string) (*models.ReviewSubmission, error) {
		return nil, repository.ErrSubmissionNotFound
	}
	created := false
	reviewRepo.CreateSubmissionFn = func(ctx context.Context, s *models.ReviewSubmission) error {
		created = true
		return nil
	}
	reviewRepo.DecrementGatedFn = func(ctx context.Context, userID string) error {
		return errors.New("counter broken")
	}
	err := ps.createReviewSubmission(context.Background(), "tpl_1", "user_1")
	require.NoError(t, err)
	require.True(t, created)
}

func TestGetReviewMetricsRequiresAdmin(t *testing.T) {
	svc, mockUser, _, _ := newReviewTestService(t)
	regular := makeAdminUser("user_1")
	regular.Role = "user"
	expectAdminAuth(mockUser, regular)

	_, err := svc.GetReviewMetrics(ContextWithUserID(context.Background(), regular.ID), &pb.GetReviewMetricsRequest{})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestGetReviewMetricsNoRepo(t *testing.T) {
	svc, mockUser, _, _ := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	expectAdminAuth(mockUser, actor)
	svc.ReviewRepo = nil

	_, err := svc.GetReviewMetrics(adminCtx(actor.ID), &pb.GetReviewMetricsRequest{})
	require.Error(t, err)
	require.Equal(t, codes.Unimplemented, status.Code(err))
}

func TestReviewAgingHoursFallback(t *testing.T) {
	svc, _, _, _ := newReviewTestService(t)
	// promptSvc is nil in this helper → falls back to the built-in default.
	require.Equal(t, defaultReviewSettings[settingAgingHours], svc.reviewAgingHours(context.Background()))
}

func TestReviewAgingHoursFromSettings(t *testing.T) {
	svc, _, _, _ := newReviewTestService(t)
	ps := NewPromptService(nil, nil, nil, nil, nil, nil, nil, nil)
	psMock := new(MockReviewRepository)
	psMock.GetSettingFn = func(ctx context.Context, key string) (string, error) {
		if key == settingAgingHours {
			return "24", nil
		}
		return "", errors.New("unexpected key")
	}
	ps.SetReviewRepo(psMock)
	svc.SetPromptService(ps)
	require.Equal(t, 24, svc.reviewAgingHours(context.Background()))
}

func TestUpdatePlatformSettingUpsertFails(t *testing.T) {
	svc, mockUser, _, mockReview := newReviewTestService(t)
	actor := makeAdminUser("admin_1")
	expectAdminAuth(mockUser, actor)

	mockReview.UpsertSettingFn = func(ctx context.Context, key, value string) error {
		return errors.New("write failed")
	}

	_, err := svc.UpdatePlatformSetting(adminCtx(actor.ID), &pb.UpdatePlatformSettingRequest{
		Key:    settingApprovalThreshold,
		Value:  "2",
		Reason: "tighten gating for new creators",
	})
	require.Error(t, err)
	require.Equal(t, codes.Internal, status.Code(err))
}
