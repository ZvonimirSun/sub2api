package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// RecordAccountProbeUsage atomically records system usage and account quota.
// The request ID makes retries idempotent; no user billing tables are touched.
func (r *accountRepository) RecordAccountProbeUsage(ctx context.Context, usage service.AccountProbeUsage) error {
	query := `WITH recorded AS (
		INSERT INTO usage_logs
		(request_id, account_id, user_id, api_key_id, model, requested_model, upstream_model, upstream_response_model,
		input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
		input_cost, output_cost, cache_creation_cost, cache_read_cost, total_cost, account_stats_cost,
		actual_cost, rate_multiplier, account_rate_multiplier, duration_ms, created_at, reasoning_effort, service_tier,
		billing_type, stream, request_type, inbound_endpoint)
		SELECT $1, id, NULL, NULL, $3, $3, $4, NULLIF($5, ''),
		$6, $7, $8, $9, $10, $11, $12, $13, $14, $14, 0, 0, $15, $16, $17, NULLIF($18, ''), NULLIF($19, ''),
		2, TRUE, 2, '/internal/modeltrace'
		FROM accounts WHERE id = $2 AND deleted_at IS NULL
		ON CONFLICT (request_id) WHERE inbound_endpoint = '/internal/modeltrace' DO NOTHING RETURNING account_id
	)
	UPDATE accounts SET extra = COALESCE(extra, '{}'::jsonb) || CASE WHEN $20::numeric > 0 THEN
		jsonb_build_object('quota_used', COALESCE((extra->>'quota_used')::numeric, 0) + $20)
		|| CASE WHEN COALESCE((extra->>'quota_daily_limit')::numeric, 0) > 0 THEN
			jsonb_build_object(
				'quota_daily_used', CASE WHEN ` + dailyExpiredExpr + ` THEN $20::numeric ELSE COALESCE((extra->>'quota_daily_used')::numeric, 0) + $20 END,
				'quota_daily_start', CASE WHEN ` + dailyExpiredExpr + ` THEN ` + nowUTC + ` ELSE COALESCE(extra->>'quota_daily_start', ` + nowUTC + `) END
			) || CASE WHEN ` + dailyExpiredExpr + ` AND ` + nextDailyResetAtExpr + ` IS NOT NULL
			THEN jsonb_build_object('quota_daily_reset_at', ` + nextDailyResetAtExpr + `) ELSE '{}'::jsonb END
		ELSE '{}'::jsonb END
		|| CASE WHEN COALESCE((extra->>'quota_weekly_limit')::numeric, 0) > 0 THEN
			jsonb_build_object(
				'quota_weekly_used', CASE WHEN ` + weeklyExpiredExpr + ` THEN $20::numeric ELSE COALESCE((extra->>'quota_weekly_used')::numeric, 0) + $20 END,
				'quota_weekly_start', CASE WHEN ` + weeklyExpiredExpr + ` THEN ` + nowUTC + ` ELSE COALESCE(extra->>'quota_weekly_start', ` + nowUTC + `) END
			) || CASE WHEN ` + weeklyExpiredExpr + ` AND ` + nextWeeklyResetAtExpr + ` IS NOT NULL
			THEN jsonb_build_object('quota_weekly_reset_at', ` + nextWeeklyResetAtExpr + `) ELSE '{}'::jsonb END
		ELSE '{}'::jsonb END
	ELSE '{}'::jsonb END, updated_at = NOW()
	WHERE id IN (SELECT account_id FROM recorded)`
	cost := usage.Cost
	if cost == nil {
		cost = &service.CostBreakdown{}
	}
	result, err := r.sql.ExecContext(ctx, query, usage.RequestID, usage.AccountID, usage.RequestedModel, usage.Model, usage.ResponseModel,
		usage.Tokens.InputTokens, usage.Tokens.OutputTokens, usage.Tokens.CacheCreationTokens, usage.Tokens.CacheReadTokens,
		cost.InputCost, cost.OutputCost, cost.CacheCreationCost, cost.CacheReadCost, usage.StandardCost,
		usage.AccountRateMultiplier, usage.DurationMs, usage.CreatedAt, usage.ReasoningEffort, usage.ServiceTier, usage.QuotaCost)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 && usage.QuotaCost > 0 {
		if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &usage.AccountID, nil, nil); err != nil {
			return err
		}
		r.syncSchedulerAccountSnapshot(ctx, usage.AccountID)
	}
	return nil
}
