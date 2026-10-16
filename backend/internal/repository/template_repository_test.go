package repository

import (
	"database/sql"
	"strings"
	"testing"

	"open-prompts/backend/internal/models"
)

func TestEscapeLike(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"100%", `100\%`},
		{"a_b", `a\_b`},
		{`back\slash`, `back\\slash`},
		{`%\_`, `\%\\\_`},
		{"邮件营销", "邮件营销"},
	}
	for _, c := range cases {
		if got := escapeLike(c.in); got != c.want {
			t.Errorf("escapeLike(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTemplateOrderBy(t *testing.T) {
	cases := map[string]string{
		"newest":                  " ORDER BY t.created_at DESC",
		"most_liked":              " ORDER BY t.like_count DESC, t.created_at DESC",
		"most_favorited":          " ORDER BY t.favorite_count DESC, t.created_at DESC",
		"trending":                " ORDER BY (COALESCE(el.cnt, 0) + COALESCE(ef.cnt, 0)) DESC, t.created_at DESC",
		"":                        " ORDER BY t.created_at DESC",
		"bogus; DROP TABLE users": " ORDER BY t.created_at DESC",
	}
	for sort, want := range cases {
		if got := templateOrderBy(sort); got != want {
			t.Errorf("templateOrderBy(%q) = %q, want %q", sort, got, want)
		}
	}
}

func TestBuildTemplateConditionsKeyword(t *testing.T) {
	filters := map[string]interface{}{"keyword": "email marketing"}
	cond, args, nextID := buildTemplateConditions(filters, nil, 2)
	if !strings.Contains(cond, "ILIKE $2") {
		t.Errorf("keyword condition should reference $2, got: %s", cond)
	}
	if len(args) != 1 {
		t.Errorf("expected 1 shared pattern arg, got %d", len(args))
	}
	want := "%email marketing%"
	for i, a := range args {
		if a != want {
			t.Errorf("arg %d = %v, want %q", i, a, want)
		}
	}
	if nextID != 3 {
		t.Errorf("next placeholder should be 3, got %d", nextID)
	}
	if !strings.Contains(cond, "unnest(t.tags)") || !strings.Contains(cond, "template_versions") {
		t.Errorf("keyword condition should match tags and latest-version content: %s", cond)
	}
}

func TestBuildTemplateConditionsEscapesMetacharacters(t *testing.T) {
	filters := map[string]interface{}{"keyword": "100%"}
	_, args, _ := buildTemplateConditions(filters, nil, 2)
	if got := args[0]; got != `%100\%%` {
		t.Errorf("escaped pattern = %q, want backslash-escaped literal", got)
	}
}

func TestBuildTemplateConditionsParameterNumbering(t *testing.T) {
	// visibility + owner consume $2/$3, so keyword should land on $4.
	filters := map[string]interface{}{
		"visibility": "public",
		"owner_id":   "u1",
		"keyword":    "kw",
	}
	cond, args, nextID := buildTemplateConditions(filters, nil, 2)
	if !strings.Contains(cond, "t.visibility = $2") || !strings.Contains(cond, "t.owner_id = $3") {
		t.Errorf("visibility/owner placeholders wrong: %s", cond)
	}
	if !strings.Contains(cond, "ILIKE $4") {
		t.Errorf("keyword should reference $4: %s", cond)
	}
	if len(args) != 3 {
		t.Errorf("expected 3 args (2 filters + 1 shared pattern), got %d", len(args))
	}
	if nextID != 5 {
		t.Errorf("next placeholder should be 5, got %d", nextID)
	}
}

func TestBuildTemplateConditionsNoKeyword(t *testing.T) {
	filters := map[string]interface{}{"visibility": "public"}
	cond, args, nextID := buildTemplateConditions(filters, nil, 2)
	if strings.Contains(cond, "ILIKE") {
		t.Errorf("no keyword filter should add no ILIKE: %s", cond)
	}
	if len(args) != 1 || nextID != 3 {
		t.Errorf("args=%v nextID=%d, want 1 arg and nextID 3", args, nextID)
	}
	if !strings.Contains(cond, "banned") {
		t.Errorf("banned-owner exclusion missing for public listing: %s", cond)
	}
}

func TestBuildTemplateConditionsModerationFilter(t *testing.T) {
	// Public streams hide gated templates except from their owner; the
	// bypass reuses the already-bound $1 (current_user_id).
	filters := map[string]interface{}{"visibility": "public"}
	cond, _, _ := buildTemplateConditions(filters, nil, 2)
	want := "AND (t.moderation_status = 'normal' OR t.owner_id = $1)"
	if !strings.Contains(cond, want) {
		t.Errorf("moderation filter missing for public listing: %s", cond)
	}
	// The filter must not add a new bound parameter (it reuses $1).
	filtersNoKw := map[string]interface{}{"visibility": "public"}
	_, args, nextID := buildTemplateConditions(filtersNoKw, nil, 2)
	if len(args) != 1 || nextID != 3 {
		t.Errorf("moderation filter must not add args: %v nextID=%d", args, nextID)
	}
}

func TestBuildTemplateConditionsModerationFilterPrivateStreamUntouched(t *testing.T) {
	// A private listing (owner's own view) must not filter by moderation
	// status — private templates are never gated.
	filters := map[string]interface{}{"visibility": "private"}
	cond, _, _ := buildTemplateConditions(filters, nil, 2)
	if strings.Contains(cond, "moderation_status") {
		t.Errorf("private listing must not filter by moderation status: %s", cond)
	}
}

func TestTemplateFromClauseTrendingJoins(t *testing.T) {
	from, args, nextID := templateFromClause("trending", []interface{}{"viewer"}, 2)
	if !strings.Contains(from, "template_likes") || !strings.Contains(from, "template_favorites") {
		t.Errorf("trending joins missing: %s", from)
	}
	if !strings.Contains(from, "$2::interval") || !strings.Contains(from, "$3::interval") {
		t.Errorf("trending window should be bound parameters: %s", from)
	}
	if len(args) != 3 || args[1] != trendingWindow || args[2] != trendingWindow {
		t.Errorf("trending args wrong: %v", args)
	}
	if nextID != 4 {
		t.Errorf("next placeholder should be 4, got %d", nextID)
	}
}

func TestTemplateFromClauseNonTrendingUnchanged(t *testing.T) {
	from, args, nextID := templateFromClause("newest", []interface{}{"viewer"}, 2)
	if strings.Contains(from, "template_likes") && strings.Contains(from, "cnt") {
		t.Errorf("non-trending sort must not add engagement joins: %s", from)
	}
	if len(args) != 1 || nextID != 2 {
		t.Errorf("non-trending should not add args: %v nextID=%d", args, nextID)
	}
}

func TestBuildTemplateConditionsExcludeWarned(t *testing.T) {
	// exclude_warned on a public stream adds a bound-parameter array filter.
	// visibility consumes $2, so the taxonomy array lands on $3.
	filters := map[string]interface{}{"visibility": "public", "exclude_warned": true}
	cond, args, nextID := buildTemplateConditions(filters, nil, 2)
	if !strings.Contains(cond, "NOT (t.content_warnings && $3)") {
		t.Errorf("exclude_warned filter missing: %s", cond)
	}
	if len(args) != 2 {
		t.Errorf("expected 2 args (viewer + taxonomy array), got %d", len(args))
	}
	if nextID != 4 {
		t.Errorf("next placeholder should be 4, got %d", nextID)
	}
}

func TestBuildTemplateConditionsExcludeWarnedPrivateUntouched(t *testing.T) {
	// exclude_warned must NOT apply to a private stream (FR-6).
	filters := map[string]interface{}{"visibility": "private", "exclude_warned": true}
	cond, _, _ := buildTemplateConditions(filters, nil, 2)
	if strings.Contains(cond, "content_warnings") {
		t.Errorf("exclude_warned must not apply to private stream: %s", cond)
	}
}

func TestBuildTemplateConditionsExcludeWarnedOff(t *testing.T) {
	// exclude_warned=false (or absent) adds no filter.
	filters := map[string]interface{}{"visibility": "public"}
	cond, _, _ := buildTemplateConditions(filters, nil, 2)
	if strings.Contains(cond, "content_warnings") {
		t.Errorf("exclude_warned off must add no filter: %s", cond)
	}
}

func TestRelatedQueryParameterOrder(t *testing.T) {
	tpl := &models.Template{
		ID:         "tpl-1",
		OwnerID:    "owner-1",
		Category:   sql.NullString{String: "Customer Support", Valid: true},
		Tags:       []string{"writing", "email"},
		Language:   "en",
		Visibility: "public",
	}
	query, args := relatedQuery("tpl-1", tpl, "viewer-1", 8)

	// Parameter order: $1 templateID, $2 viewer, $3 category, $4 tags,
	// $5 language, $6 owner, $7 limit.
	if len(args) != 7 {
		t.Fatalf("expected 7 args, got %d: %v", len(args), args)
	}
	if args[0] != "tpl-1" {
		t.Errorf("arg[0] (templateID) = %v", args[0])
	}
	if args[1] != "viewer-1" {
		t.Errorf("arg[1] (viewer) = %v", args[1])
	}
	if args[2] != "Customer Support" {
		t.Errorf("arg[2] (category) = %v", args[2])
	}
	if args[3] == nil {
		t.Errorf("arg[3] (tags) should be a pq.Array, got nil")
	}
	if args[4] != "en" {
		t.Errorf("arg[4] (language) = %v", args[4])
	}
	if args[5] != "owner-1" {
		t.Errorf("arg[5] (owner) = %v", args[5])
	}
	if args[6] != 8 {
		t.Errorf("arg[6] (limit) = %v", args[6])
	}

	// The query must reference all seven placeholders and exclude the
	// current template + banned owners.
	for _, ph := range []string{"$1", "$2", "$3", "$4", "$5", "$6", "$7"} {
		if !strings.Contains(query, ph) {
			t.Errorf("query missing placeholder %s", ph)
		}
	}
	if !strings.Contains(query, "t.id != $1") {
		t.Errorf("query must exclude the current template")
	}
	if !strings.Contains(query, "status = 'banned'") {
		t.Errorf("query must exclude banned owners")
	}
	if !strings.Contains(query, "t.visibility = 'public'") {
		t.Errorf("query must filter public visibility")
	}
}

func TestRelatedQueryEmptyTagsEdge(t *testing.T) {
	// A template with no tags must still produce a valid query (tags && '{}'
	// is false -> no points, no error).
	tpl := &models.Template{
		ID:         "tpl-1",
		OwnerID:    "owner-1",
		Category:   sql.NullString{String: "", Valid: false},
		Tags:       []string{},
		Language:   "en",
		Visibility: "public",
	}
	query, args := relatedQuery("tpl-1", tpl, "", 8)
	if len(args) != 7 {
		t.Fatalf("expected 7 args, got %d", len(args))
	}
	if args[2] != "" {
		t.Errorf("empty category should bind empty string, got %v", args[2])
	}
	if !strings.Contains(query, "t.tags && $4") {
		t.Errorf("tags overlap clause missing: %s", query)
	}
}

func TestAuthorQueryParameterOrder(t *testing.T) {
	query, args := authorQuery("tpl-1", "owner-1", "viewer-1", 4)
	if len(args) != 4 {
		t.Fatalf("expected 4 args, got %d: %v", len(args), args)
	}
	if args[0] != "tpl-1" {
		t.Errorf("arg[0] (templateID) = %v", args[0])
	}
	if args[1] != "viewer-1" {
		t.Errorf("arg[1] (viewer) = %v", args[1])
	}
	if args[2] != "owner-1" {
		t.Errorf("arg[2] (owner) = %v", args[2])
	}
	if args[3] != 4 {
		t.Errorf("arg[3] (limit) = %v", args[3])
	}
	for _, ph := range []string{"$1", "$2", "$3", "$4"} {
		if !strings.Contains(query, ph) {
			t.Errorf("query missing placeholder %s", ph)
		}
	}
	if !strings.Contains(query, "t.owner_id = $3") {
		t.Errorf("author query must filter by owner")
	}
	if !strings.Contains(query, "t.id != $1") {
		t.Errorf("author query must exclude the current template")
	}
	if !strings.Contains(query, "t.like_count DESC") {
		t.Errorf("author query must order by like_count DESC")
	}
}
