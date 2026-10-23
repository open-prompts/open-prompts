package service

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHardDeleteTemplate(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	svc := NewTemplateCascadeService(db)

	mock.ExpectBegin()
	for _, table := range []string{
		"template_versions",
		"template_aliases",
		"template_comments",
		"template_likes",
		"template_favorites",
		"template_ratings",
	} {
		mock.ExpectExec("DELETE FROM " + table + " WHERE template_id = \\$1").
			WithArgs("tpl1").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectExec("DELETE FROM templates WHERE id = \\$1").
		WithArgs("tpl1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = svc.HardDeleteTemplate(context.Background(), "tpl1")
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestHardDeleteTemplateRollbackOnError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	svc := NewTemplateCascadeService(db)

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM template_versions WHERE template_id = \\$1").
		WithArgs("tpl1").WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()

	err = svc.HardDeleteTemplate(context.Background(), "tpl1")
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}