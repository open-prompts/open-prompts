package service

import (
	"context"
	"database/sql"
	"time"

	"go.uber.org/zap"
)

// softDeleteScanInterval is how often the 30-day hard-delete due-scan runs.
// A 24h interval matches the design; the scan is idempotent and catch-up safe,
// so a crash mid-batch is safe to re-run.
const softDeleteScanInterval = 24 * time.Hour

// TickerService runs the background due-scan jobs (currently the 30-day
// hard-delete of expired soft-deleted templates). It follows the
// scheduler-sharing/webhooks DB-backed due-scan pattern.
type TickerService struct {
	db      *sql.DB
	cascade *TemplateCascadeService
	stop    chan struct{}
}

// NewTickerService creates a TickerService.
func NewTickerService(db *sql.DB, cascade *TemplateCascadeService) *TickerService {
	return &TickerService{
		db:      db,
		cascade: cascade,
		stop:    make(chan struct{}),
	}
}

// Start launches the background scan loop. It runs once on startup (catch-up
// safe) and then on the configured interval until Stop is called.
func (s *TickerService) Start(ctx context.Context) {
	go func() {
		s.scanExpiredSoftDeletes(ctx)
		ticker := time.NewTicker(softDeleteScanInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stop:
				return
			case <-ticker.C:
				s.scanExpiredSoftDeletes(ctx)
			}
		}
	}()
}

// Stop halts the background loop.
func (s *TickerService) Stop() {
	close(s.stop)
}

// scanExpiredSoftDeletes hard-deletes soft_deleted templates past the restore
// window. Idempotent: the WHERE moderation_status = 'soft_deleted' guard means
// a restored template is never picked up; a crash mid-batch is safe to re-run.
func (s *TickerService) scanExpiredSoftDeletes(ctx context.Context) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM templates
		WHERE moderation_status = 'soft_deleted'
		  AND deleted_at < now() - ($1::int * interval '1 day')
		LIMIT 1000`, modSoftDeleteWindowDays())
	if err != nil {
		zap.S().Errorf("TickerService: failed to scan expired soft-deletes: %v", err)
		return
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			zap.S().Errorf("TickerService: failed to scan row: %v", err)
			return
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		zap.S().Errorf("TickerService: rows error: %v", err)
		return
	}

	for _, id := range ids {
		if err := s.cascade.HardDeleteTemplate(ctx, id); err != nil {
			zap.S().Errorf("TickerService: hard-delete failed (template=%s): %v", id, err)
			continue
		}
		zap.S().Infof("TickerService: hard-deleted expired soft-deleted template %s", id)
	}
}
