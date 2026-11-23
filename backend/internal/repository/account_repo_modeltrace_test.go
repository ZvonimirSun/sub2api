package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestModelTraceAccountQueryFiltersEnrollmentWithoutLimit(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_, query string) error {
		if !strings.Contains(query, `jsonb_typeof("accounts"."extra"->'modeltrace') = 'object'`) ||
			!strings.Contains(query, `"deleted_at" IS NULL`) || strings.Contains(query, "LIMIT") {
			return fmt.Errorf("unexpected monitor query: %s", query)
		}
		return nil
	})))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	repo := &accountRepository{client: client}
	rateLimitAt := time.Now().Add(5 * time.Minute).UTC()
	mock.ExpectQuery("monitored accounts").WillReturnRows(sqlmock.NewRows([]string{
		"id", "name", "platform", "type", "status",
		"schedulable", "rate_limit_reset_at", "overload_until", "temp_unschedulable_until", "expires_at", "auto_pause_on_expired",
		"extra",
	}).
		AddRow(1002, "monitored", "openai", "apikey", "active",
			false, rateLimitAt, nil, nil, nil, false,
			[]byte(`{"modeltrace":{"models":{"gpt-5.4":{}}}}`)))
	accounts, err := repo.ListModelTraceAccounts(context.Background())
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Equal(t, int64(1002), accounts[0].ID)
	require.False(t, accounts[0].Schedulable)
	require.NotNil(t, accounts[0].RateLimitResetAt)
	require.False(t, accounts[0].IsSchedulable())
	require.NotNil(t, accounts[0].Extra["modeltrace"])
	require.NoError(t, mock.ExpectationsWereMet())
}
