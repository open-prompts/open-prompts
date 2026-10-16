package service

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"go.uber.org/zap"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AnalyticsRepo is the data-access contract the analytics service methods
// depend on. *repository.AnalyticsRepository satisfies it; tests use a mock.
type AnalyticsRepo interface {
	InsertEvent(ctx context.Context, templateID, eventType, source, userID, sessionID string) error
	RecentViewExists(ctx context.Context, templateID, sessionID string, window time.Duration) (bool, error)
	IncrementCounter(ctx context.Context, templateID, column string) error
	DailyEventSeries(ctx context.Context, templateID, ownerID string, from, to time.Time) (map[string][2]int32, error)
	DailyLikeSeries(ctx context.Context, templateID, ownerID string, from, to time.Time) (map[string]int32, error)
	DailyFavoriteSeries(ctx context.Context, templateID, ownerID string, from, to time.Time) (map[string]int32, error)
	UseSourceSplit(ctx context.Context, templateID, ownerID string, from, to time.Time) (web, api int32, err error)
	TopTemplatesByUses(ctx context.Context, ownerID string, from, to time.Time, limit int) ([]models.TopTemplateRow, error)
	CountOwnerTemplates(ctx context.Context, ownerID string) (int32, error)
}

// tplEventRatePerMinDef is the default per-IP view-event rate limit.
const tplEventRatePerMinDef = 60

// viewDedupWindow is how long a (template, session) view is deduplicated.
const viewDedupWindow = 30 * time.Minute

// validAnalyticsDays is the whitelist of accepted range lengths.
var validAnalyticsDays = map[int32]bool{7: true, 30: true, 90: true}

// getUserFromContext returns the caller's user id, or "" for anonymous.
func getUserFromContext(ctx context.Context) string {
	uid, _ := GetUserIDFromContext(ctx)
	return uid
}

// RecordTemplateEvent records a client-side view event (anonymous allowed,
// rate-limited per IP). Use events are recorded server-side only.
func (s *PromptService) RecordTemplateEvent(ctx context.Context, req *pb.RecordTemplateEventRequest) (*pb.RecordTemplateEventResponse, error) {
	if req.TemplateId == "" {
		return nil, status.Error(codes.InvalidArgument, "template id required")
	}
	if req.EventType != "view" {
		return nil, status.Error(codes.InvalidArgument, "unsupported event type")
	}
	if retry, ok := s.checkEventRate(ctx); !ok {
		return nil, status.Errorf(codes.ResourceExhausted, "rate limited; retry after %d seconds", retry)
	}

	tpl, err := s.TemplateRepo.Get(ctx, req.TemplateId, "")
	if err != nil {
		return nil, status.Error(codes.NotFound, "template not found")
	}
	caller := getUserFromContext(ctx)
	if tpl.Visibility == "private" && caller != tpl.OwnerID {
		return nil, status.Error(codes.NotFound, "template not found")
	}

	// Dedup: skip when a recent view for (template, session) exists.
	if req.SessionId != "" {
		dup, err := s.viewIsDuplicate(ctx, req.TemplateId, req.SessionId)
		if err == nil && dup {
			return &pb.RecordTemplateEventResponse{}, nil
		}
	}

	if s.AnalyticsRepo == nil {
		return nil, status.Error(codes.Internal, "analytics unavailable")
	}
	if err := s.AnalyticsRepo.InsertEvent(ctx, req.TemplateId, "view", "web", caller, req.SessionId); err != nil {
		zap.S().Errorf("failed to insert view event: %v", err)
		return nil, status.Error(codes.Internal, "failed to record event")
	}
	// Counter drift is acceptable; the event table is the source of truth.
	if err := s.AnalyticsRepo.IncrementCounter(ctx, req.TemplateId, "view_count"); err != nil {
		zap.S().Errorf("failed to increment view_count: %v", err)
	}
	return &pb.RecordTemplateEventResponse{}, nil
}

// RecordUseEvent records a use event (internal; not an RPC). Never returns
// an error; callers run it in a goroutine with context.Background().
func (s *PromptService) RecordUseEvent(ctx context.Context, templateID, userID, source string) {
	if templateID == "" {
		return
	}
	if source != "web" && source != "api" {
		return
	}
	if s.AnalyticsRepo == nil {
		return
	}
	if err := s.AnalyticsRepo.InsertEvent(ctx, templateID, "use", source, userID, ""); err != nil {
		zap.S().Errorf("failed to insert use event: %v", err)
		return
	}
	if err := s.AnalyticsRepo.IncrementCounter(ctx, templateID, "use_count"); err != nil {
		zap.S().Errorf("failed to increment use_count: %v", err)
	}
}

// checkEventRate enforces TEMPLATE_EVENT_RATE_PER_MINUTE per client IP using
// a Redis fixed-window counter (key tpl_event_rate:{ip}). Fail-open when
// Redis is unavailable. Returns (retryAfterSeconds, allowed).
func (s *PromptService) checkEventRate(ctx context.Context) (int, bool) {
	if s.Redis == nil {
		return 0, true
	}
	ip := clientIPFromContext(ctx)
	key := "tpl_event_rate:" + ip
	max, _ := strconv.Atoi(os.Getenv("TEMPLATE_EVENT_RATE_PER_MINUTE"))
	if max <= 0 {
		max = tplEventRatePerMinDef
	}
	window := time.Minute

	v, err := s.Redis.Get(ctx, key)
	if err != nil || v == "" {
		_ = s.Redis.Set(ctx, key, "1:"+strconv.FormatInt(time.Now().Add(window).Unix(), 10), window)
		return 0, true
	}
	var count int
	var expires int64
	if _, err := fmt.Sscanf(v, "%d:%d", &count, &expires); err != nil {
		_ = s.Redis.Set(ctx, key, "1:"+strconv.FormatInt(time.Now().Add(window).Unix(), 10), window)
		return 0, true
	}
	if time.Now().Unix() >= expires { // window elapsed: reset
		_ = s.Redis.Set(ctx, key, "1:"+strconv.FormatInt(time.Now().Add(window).Unix(), 10), window)
		return 0, true
	}
	if count >= max {
		retry := int(expires - time.Now().Unix())
		if retry < 1 {
			retry = 1
		}
		return retry, false
	}
	_ = s.Redis.Set(ctx, key,
		strconv.Itoa(count+1)+":"+strconv.FormatInt(expires, 10),
		time.Duration(expires-time.Now().Unix())*time.Second)
	return 0, true
}

// viewIsDuplicate reports whether a recent view for (template, session)
// exists. Uses Redis SETNX; falls back to the DB when Redis is unavailable.
func (s *PromptService) viewIsDuplicate(ctx context.Context, templateID, sessionID string) (bool, error) {
	key := "tpl_view_dedup:" + templateID + ":" + sessionID
	if s.Redis != nil {
		ok, err := s.Redis.SetNX(ctx, key, "1", viewDedupWindow)
		if err == nil {
			return !ok, nil
		}
		// Redis down → DB fallback below.
	}
	if s.AnalyticsRepo == nil {
		return false, nil
	}
	exists, err := s.AnalyticsRepo.RecentViewExists(ctx, templateID, sessionID, viewDedupWindow)
	if err != nil {
		// DB error: count it (brief over-counting is acceptable).
		return false, nil
	}
	return exists, nil
}

// GetTemplateStats returns analytics for a single template (owner only).
func (s *PromptService) GetTemplateStats(ctx context.Context, req *pb.GetTemplateStatsRequest) (*pb.GetTemplateStatsResponse, error) {
	caller := getUserFromContext(ctx)
	if caller == "" {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	if !validAnalyticsDays[req.Days] {
		return nil, status.Error(codes.InvalidArgument, "days must be 7, 30, or 90")
	}
	tpl, err := s.TemplateRepo.Get(ctx, req.TemplateId, caller)
	if err != nil {
		return nil, status.Error(codes.NotFound, "template not found")
	}
	if tpl.OwnerID != caller {
		if tpl.Visibility == "private" {
			return nil, status.Error(codes.NotFound, "template not found")
		}
		return nil, status.Error(codes.PermissionDenied, "not the template owner")
	}
	if s.AnalyticsRepo == nil {
		return nil, status.Error(codes.Internal, "analytics unavailable")
	}

	to := time.Now().UTC()
	from := to.AddDate(0, 0, -int(req.Days))
	prevTo := from
	prevFrom := from.AddDate(0, 0, -int(req.Days))

	evCur, err := s.AnalyticsRepo.DailyEventSeries(ctx, req.TemplateId, "", from, to)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load analytics")
	}
	lkCur, err := s.AnalyticsRepo.DailyLikeSeries(ctx, req.TemplateId, "", from, to)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load analytics")
	}
	fvCur, err := s.AnalyticsRepo.DailyFavoriteSeries(ctx, req.TemplateId, "", from, to)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load analytics")
	}
	evPrev, err := s.AnalyticsRepo.DailyEventSeries(ctx, req.TemplateId, "", prevFrom, prevTo)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load analytics")
	}
	lkPrev, err := s.AnalyticsRepo.DailyLikeSeries(ctx, req.TemplateId, "", prevFrom, prevTo)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load analytics")
	}
	fvPrev, err := s.AnalyticsRepo.DailyFavoriteSeries(ctx, req.TemplateId, "", prevFrom, prevTo)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load analytics")
	}
	web, api, err := s.AnalyticsRepo.UseSourceSplit(ctx, req.TemplateId, "", from, to)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load analytics")
	}

	series := zeroFill(int(req.Days), from, evCur, lkCur, fvCur)
	current := sumSeries(series)
	previous := sumSeries(zeroFill(int(req.Days), prevFrom, evPrev, lkPrev, fvPrev))
	rate := 0.0
	if current.Views > 0 {
		rate = float64(current.Uses) / float64(current.Views)
	}

	return &pb.GetTemplateStatsResponse{
		Current:       current,
		Previous:      previous,
		Series:        series,
		SourceSplit:   &pb.SourceSplit{Web: web, Api: api},
		ViewToUseRate: rate,
	}, nil
}

// GetCreatorStats returns aggregate analytics for the caller's templates.
func (s *PromptService) GetCreatorStats(ctx context.Context, req *pb.GetCreatorStatsRequest) (*pb.GetCreatorStatsResponse, error) {
	caller := getUserFromContext(ctx)
	if caller == "" {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	if !validAnalyticsDays[req.Days] {
		return nil, status.Error(codes.InvalidArgument, "days must be 7, 30, or 90")
	}
	if s.AnalyticsRepo == nil {
		return nil, status.Error(codes.Internal, "analytics unavailable")
	}

	to := time.Now().UTC()
	from := to.AddDate(0, 0, -int(req.Days))
	prevTo := from
	prevFrom := from.AddDate(0, 0, -int(req.Days))

	evCur, err := s.AnalyticsRepo.DailyEventSeries(ctx, "", caller, from, to)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load analytics")
	}
	lkCur, err := s.AnalyticsRepo.DailyLikeSeries(ctx, "", caller, from, to)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load analytics")
	}
	fvCur, err := s.AnalyticsRepo.DailyFavoriteSeries(ctx, "", caller, from, to)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load analytics")
	}
	evPrev, err := s.AnalyticsRepo.DailyEventSeries(ctx, "", caller, prevFrom, prevTo)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load analytics")
	}
	lkPrev, err := s.AnalyticsRepo.DailyLikeSeries(ctx, "", caller, prevFrom, prevTo)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load analytics")
	}
	fvPrev, err := s.AnalyticsRepo.DailyFavoriteSeries(ctx, "", caller, prevFrom, prevTo)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load analytics")
	}
	web, api, err := s.AnalyticsRepo.UseSourceSplit(ctx, "", caller, from, to)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load analytics")
	}
	top, err := s.AnalyticsRepo.TopTemplatesByUses(ctx, caller, from, to, 10)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load analytics")
	}
	count, err := s.AnalyticsRepo.CountOwnerTemplates(ctx, caller)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load analytics")
	}

	series := zeroFill(int(req.Days), from, evCur, lkCur, fvCur)
	current := sumSeries(series)
	previous := sumSeries(zeroFill(int(req.Days), prevFrom, evPrev, lkPrev, fvPrev))

	topRows := make([]*pb.TopTemplateRow, 0, len(top))
	for _, row := range top {
		rate := 0.0
		if row.Views > 0 {
			rate = float64(row.Uses) / float64(row.Views)
		}
		topRows = append(topRows, &pb.TopTemplateRow{
			Id:        row.ID,
			Title:     row.Title,
			Views:     row.Views,
			Uses:      row.Uses,
			Likes:     row.Likes,
			Favorites: row.Favorites,
			Rate:      rate,
		})
	}

	return &pb.GetCreatorStatsResponse{
		Current:       current,
		Previous:      previous,
		Series:        series,
		SourceSplit:   &pb.SourceSplit{Web: web, Api: api},
		TopTemplates:  topRows,
		TemplateCount: count,
	}, nil
}

// zeroFill builds exactly `days` daily points, oldest first, filling missing
// days with zeros from the per-day maps.
func zeroFill(days int, from time.Time, ev map[string][2]int32, lk, fv map[string]int32) []*pb.DailyMetricPoint {
	points := make([]*pb.DailyMetricPoint, 0, days)
	for i := 0; i < days; i++ {
		day := from.AddDate(0, 0, i).Format("2006-01-02")
		views, uses := int32(0), int32(0)
		if v, ok := ev[day]; ok {
			views, uses = v[0], v[1]
		}
		likes := lk[day]
		favorites := fv[day]
		points = append(points, &pb.DailyMetricPoint{
			Date:      day,
			Views:     views,
			Uses:      uses,
			Likes:     likes,
			Favorites: favorites,
		})
	}
	return points
}

// sumSeries totals a zero-filled daily series into a summary.
func sumSeries(series []*pb.DailyMetricPoint) *pb.TemplateStatsSummary {
	sum := &pb.TemplateStatsSummary{}
	for _, p := range series {
		sum.Views += p.Views
		sum.Uses += p.Uses
		sum.Likes += p.Likes
		sum.Favorites += p.Favorites
	}
	return sum
}