package service

import (
	"context"
	"sync"
	"time"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"
	"open-prompts/backend/internal/repository"

	"go.uber.org/zap"
)

// prefsCacheTTL bounds the in-process notification-preferences cache
// staleness (architecture §3 Q2). A short TTL keeps suppression near-real-time
// while avoiding a DB read per engagement event.
const prefsCacheTTL = 30 * time.Second

// prefsCache is a short-TTL in-process cache keyed by recipientID. It is
// invalidated on PATCH so a user's change takes effect immediately (FR-5).
type prefsCache struct {
	mu      sync.Mutex
	entries map[string]prefsCacheEntry
}

type prefsCacheEntry struct {
	prefs     *models.NotificationPreferences
	found     bool
	expiresAt time.Time
}

var notifPrefsCache = &prefsCache{entries: make(map[string]prefsCacheEntry)}

// get returns the cached entry for a recipient, if present and unexpired.
func (c *prefsCache) get(userID string) (*models.NotificationPreferences, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[userID]
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false, false
	}
	return e.prefs, e.found, true
}

// set stores an entry for a recipient.
func (c *prefsCache) set(userID string, prefs *models.NotificationPreferences, found bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[userID] = prefsCacheEntry{
		prefs:     prefs,
		found:     found,
		expiresAt: time.Now().Add(prefsCacheTTL),
	}
}

// invalidate removes a recipient's cached entry (called on PATCH).
func (c *prefsCache) invalidate(userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, userID)
}

// prefsAllow reports whether the recipient should receive a notification of
// the given type. Absent row = allowed. Paused-all overrides everything.
// Fail-open on repository error (log + allow) so a prefs outage never blocks
// engagement notifications. Reads through the short-TTL in-process cache.
func prefsAllow(ctx context.Context, repo repository.NotificationPreferencesRepository, recipientID, notificationType string) bool {
	if repo == nil {
		return true
	}
	if prefs, found, ok := notifPrefsCache.get(recipientID); ok {
		return prefsAllowFrom(prefs, found, notificationType)
	}
	prefs, found, err := repo.Get(ctx, recipientID)
	if err != nil {
		zap.S().Warnf("prefs check failed for %s: %v", recipientID, err)
		return true // fail-open
	}
	notifPrefsCache.set(recipientID, prefs, found)
	return prefsAllowFrom(prefs, found, notificationType)
}

// prefsAllowFrom applies the effective-prefs resolution logic to a loaded
// (or defaulted) preference row. Shared by the emission-time check and the
// effective-prefs read so both use one code path (NFR-3).
func prefsAllowFrom(prefs *models.NotificationPreferences, found bool, notificationType string) bool {
	if !found {
		return true // defaults: all enabled
	}
	switch notificationType {
	case "like":
		if prefs.PausedAll {
			return false
		}
		return prefs.LikeEnabled
	case "favorite":
		if prefs.PausedAll {
			return false
		}
		return prefs.FavoriteEnabled
	case "fork":
		if prefs.PausedAll {
			return false
		}
		return prefs.ForkEnabled
	case "comment", "comment_reply":
		if prefs.PausedAll {
			return false
		}
		return prefs.CommentEnabled
	case "follow":
		if prefs.PausedAll {
			return false
		}
		return prefs.FollowEnabled
	default:
		// Admin/system types (moderation, appeal_resolved, template_approved,
		// template_rejected, template_warning_changed) are NOT suppressed in
		// v1 (FR-10), even when paused_all is on.
		return true
	}
}

// prefsToProto maps effective preferences to the proto response, applying
// defaults when no row exists (all booleans true, paused_all false).
func prefsToProto(prefs *models.NotificationPreferences, found bool) *pb.NotificationPreferences {
	if !found {
		return &pb.NotificationPreferences{
			LikeEnabled:     true,
			FavoriteEnabled: true,
			ForkEnabled:     true,
			CommentEnabled:  true,
			FollowEnabled:   true,
			PausedAll:       false,
		}
	}
	return &pb.NotificationPreferences{
		LikeEnabled:     prefs.LikeEnabled,
		FavoriteEnabled: prefs.FavoriteEnabled,
		ForkEnabled:     prefs.ForkEnabled,
		CommentEnabled:  prefs.CommentEnabled,
		FollowEnabled:   prefs.FollowEnabled,
		PausedAll:       prefs.PausedAll,
	}
}
