package admin

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestCodexTurnStateSnapshotUsesLatestObservationAndPinTime(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	now := time.Date(2026, 9, 18, 1, 30, 0, 0, time.UTC)
	mock.ExpectQuery("upstream_model_mismatch IS TRUE").WillReturnRows(codexTurnStateDegradedRows().
		AddRow(int64(7), "sample", "gpt-x", "gpt-x", "gpt-y", int64(4), now, now.Add(-time.Hour), nil).
		AddRow(int64(7), "sample", "gpt-x", "gpt-x", "gpt-z", int64(1), now, now, nil))
	h := &CodexTurnStatePanelHandler{degraded: NewCodexTurnStateDegradedHandler(db)}
	result := h.enrichSnapshot(context.Background(), []byte(`{"accounts":[{"id":7,"models":[{"model":"GPT-X","pinned_updated_at":"2026-09-18T01:00:00Z"},{"model":"gpt-q"}]}]}`))
	var state map[string]any
	require.NoError(t, json.Unmarshal(result, &state))
	accounts, ok := state["accounts"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, accounts)
	account, ok := accounts[0].(map[string]any)
	require.True(t, ok)
	models, ok := account["models"].([]any)
	require.True(t, ok)
	require.Len(t, models, 2)
	firstModel, ok := models[0].(map[string]any)
	require.True(t, ok)
	secondModel, ok := models[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "degraded", firstModel["degradation"])
	require.Equal(t, "no_record", secondModel["degradation"])
	require.Equal(t, true, state["degraded_enabled"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCodexTurnStateSnapshotQueryFailureIsUnknown(t *testing.T) {
	h := &CodexTurnStatePanelHandler{degraded: NewCodexTurnStateDegradedHandler(nil)}
	result := h.enrichSnapshot(context.Background(), []byte(`{"accounts":[{"id":7,"models":[{"model":"gpt-x"}]}]}`))
	require.Contains(t, string(result), `"degradation":"unknown"`)
	require.Contains(t, string(result), `"degraded_error":"unavailable"`)
	require.Equal(t, "null", string(h.enrichSnapshot(context.Background(), []byte(`null`))))
}
