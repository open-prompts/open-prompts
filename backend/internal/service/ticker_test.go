package service

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanExpiredSoftDeletes(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	cascade := NewTemplateCascadeService(db)
	svc := NewTickerService(db, cascade)

	// Two expired templates are hard-deleted.
	mock.ExpectQuery("SELECT id FROM templates").
		WithArgs(30).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("t1").AddRow("t2"))

	mock.ExpectBegin()
	for _, table := range []string{
		"template_versions", "template_aliases", "template_comments",
		"template_likes", "template_favorites", "template_ratings",
	} {
		mock.ExpectExec("DELETE FROM " + table + " WHERE template_id = \\$1").
			WithArgs("t1").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectExec("DELETE FROM templates WHERE id = \\$1").
		WithArgs("t1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	mock.ExpectBegin()
	for _, table := range []string{
		"template_versions", "template_aliases", "template_comments",
		"template_likes", "template_favorites", "template_ratings",
	} {
		mock.ExpectExec("DELETE FROM " + table + " WHERE template_id = \\$1").
			WithArgs("t2").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectExec("DELETE FROM templates WHERE id = \\$1").
		WithArgs("t2").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	svc.scanExpiredSoftDeletes(context.Background())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestScanExpiredSoftDeletesQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	cascade := NewTemplateCascadeService(db)
	svc := NewTickerService(db, cascade)

	mock.ExpectQuery("SELECT id FROM templates").
		WithArgs(30).WillReturnError(assert.AnError)

	svc.scanExpiredSoftDeletes(context.Background())
	assert.NoError(t, mock.ExpectationsWereMet())
}