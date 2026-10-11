package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// AccountProbeUsage deliberately has no user, subscription or API key identity.
type AccountProbeUsage struct {
	RequestID             string
	AccountID             int64
	Model                 string
	RequestedModel        string
	ResponseModel         string
	Tokens                UsageTokens
	Cost                  *CostBreakdown
	AccountRateMultiplier float64
	DurationMs            int64
	ReasoningEffort       string
	ServiceTier           string
	StandardCost          float64
	AccountCost           float64
	QuotaCost             float64
	CreatedAt             time.Time
}

type accountProbeUsageWriter interface {
	RecordAccountProbeUsage(context.Context, AccountProbeUsage) error
}

type modelTraceUsageKey struct{}
type modelTraceUsageCollector struct {
	tokens      UsageTokens
	model       string
	serviceTier string
	seen        bool
	startedAt   time.Time
}

// Usage events are cumulative snapshots (Anthropic splits input and output),
// so repeated terminal events must not count the same tokens twice.
func observeModelTraceUsage(ctx context.Context, payload string) {
	collector, ok := ctx.Value(modelTraceUsageKey{}).(*modelTraceUsageCollector)
	if !ok {
		return
	}
	root := gjson.Parse(payload)
	response := root.Get("response")
	if !response.Exists() {
		response = root
	}
	if model := response.Get("model").String(); model != "" {
		collector.model = model
	}
	if tier := response.Get("service_tier").String(); tier != "" {
		collector.serviceTier = tier
	}
	if usage, ok := extractOpenAIUsageFromJSONBytes([]byte(payload)); ok {
		collector.seen = true
		collector.tokens.InputTokens = max(collector.tokens.InputTokens, max(usage.InputTokens-usage.CacheReadInputTokens-usage.CacheCreationInputTokens, 0))
		collector.tokens.OutputTokens = max(collector.tokens.OutputTokens, usage.OutputTokens)
		collector.tokens.CacheReadTokens = max(collector.tokens.CacheReadTokens, usage.CacheReadInputTokens)
		collector.tokens.CacheCreationTokens = max(collector.tokens.CacheCreationTokens, usage.CacheCreationInputTokens)
	}
	// Anthropic input_tokens already excludes cache tokens.
	for _, path := range []string{"message.usage", "usage"} {
		usage := root.Get(path)
		if root.Get("type").String() == "message_start" || root.Get("type").String() == "message_delta" {
			if usage.Exists() {
				collector.seen = true
				collector.tokens.InputTokens = max(collector.tokens.InputTokens, int(usage.Get("input_tokens").Int()))
				collector.tokens.OutputTokens = max(collector.tokens.OutputTokens, int(usage.Get("output_tokens").Int()))
				collector.tokens.CacheReadTokens = max(collector.tokens.CacheReadTokens, int(usage.Get("cache_read_input_tokens").Int()))
				collector.tokens.CacheCreationTokens = max(collector.tokens.CacheCreationTokens, int(usage.Get("cache_creation_input_tokens").Int()))
			}
		}
	}
	if usage := response.Get("usageMetadata"); usage.Exists() {
		collector.seen = true
		cached := int(usage.Get("cachedContentTokenCount").Int())
		collector.tokens.InputTokens = max(collector.tokens.InputTokens, max(int(usage.Get("promptTokenCount").Int())-cached, 0))
		collector.tokens.CacheReadTokens = max(collector.tokens.CacheReadTokens, cached)
		collector.tokens.OutputTokens = max(collector.tokens.OutputTokens, int(usage.Get("candidatesTokenCount").Int()+usage.Get("thoughtsTokenCount").Int()))
	}
}

func (s *ModelTraceService) recordProbeUsage(ctx context.Context, account *Account, model string, effort string, usage *modelTraceUsageCollector) error {
	writer, ok := s.accountRepo.(accountProbeUsageWriter)
	if !ok {
		return fmt.Errorf("account probe usage writer unavailable")
	}
	entry := AccountProbeUsage{RequestID: uuid.NewString(), AccountID: account.ID, Model: account.GetMappedModel(model), RequestedModel: model, ResponseModel: usage.model, Tokens: usage.tokens, CreatedAt: usage.startedAt, DurationMs: time.Since(usage.startedAt).Milliseconds(), ReasoningEffort: effort, ServiceTier: usage.serviceTier, AccountRateMultiplier: account.BillingRateMultiplier()}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
		entry.DurationMs = 0
	}
	if account.IsOpenAIOAuthLike() {
		entry.Model = normalizeOpenAIModelForUpstream(account, entry.Model)
	}
	pricing := NewBillingService(s.cfg, nil)
	var resolver *ModelPricingResolver
	if s.accountTestSvc != nil && s.accountTestSvc.openaiGatewayService != nil {
		gateway := s.accountTestSvc.openaiGatewayService
		if gateway.billingService != nil {
			pricing = gateway.billingService
		}
		resolver = gateway.resolver
	}
	cost, err := pricing.CalculateCostUnified(CostInput{Ctx: ctx, Model: entry.Model, Tokens: entry.Tokens, RateMultiplier: 1, RequestCount: 1, PricingAt: entry.CreatedAt, ServiceTier: usage.serviceTier, ReasoningEffort: effort, Resolver: resolver})
	if err != nil {
		logger.FromContext(ctx).Warn("modeltrace.usage_pricing_failed", zap.Error(err))
		// Preserve actual tokens even if the upstream reports an unknown model.
	} else {
		entry.Cost = cost
		entry.StandardCost = cost.TotalCost
		entry.AccountCost = cost.TotalCost * account.BillingRateMultiplier()
		if account.IsAPIKeyOrBedrock() && account.HasAnyQuotaLimit() {
			entry.QuotaCost = entry.AccountCost
		}
	}
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return writer.RecordAccountProbeUsage(saveCtx, entry)
}
