package service

import (
	"regexp"
	"strings"
	"time"
)

const (
	DegradedAccountsDefaultWindow = 30 * time.Minute
	DegradedAccountsMinWindow     = time.Minute
	DegradedAccountsMaxWindow     = 6 * time.Hour

	degradedAccountsWindowMaxLength = 16
)

// DegradedAccount 表示一个发生模型降级或不一致的聚合槽位。
type DegradedAccount struct {
	AccountID      int64     `json:"account_id"`
	AccountName    string    `json:"account_name"`
	RequestedModel string    `json:"requested_model"`
	SentModel      string    `json:"sent_model"`
	ResponseModel  string    `json:"response_model"`
	Count          int64     `json:"count"`
	FirstSeen      time.Time `json:"first_seen"`
	LastSeen       time.Time `json:"last_seen"`
	TTFTAvgMs      *float64  `json:"ttft_avg_ms"`
}

var (
	modelVariantLatestSuffix      = regexp.MustCompile(`-latest$`)
	modelVariantISODateSuffix     = regexp.MustCompile(`-\d{4}-\d{2}-\d{2}$`)
	modelVariantCompactDateSuffix = regexp.MustCompile(`-\d{8}$`)
)

// ParseDegradedAccountsWindow 解析 ?window= 查询参数。
func ParseDegradedAccountsWindow(raw string) (time.Duration, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DegradedAccountsDefaultWindow, true
	}
	if len(raw) > degradedAccountsWindowMaxLength {
		return 0, false
	}
	window, err := time.ParseDuration(raw)
	if err != nil || window <= 0 {
		return 0, false
	}
	return ClampDegradedAccountsWindow(window), true
}

// ClampDegradedAccountsWindow 限制窗口范围。
func ClampDegradedAccountsWindow(window time.Duration) time.Duration {
	if window < DegradedAccountsMinWindow {
		return DegradedAccountsMinWindow
	}
	if window > DegradedAccountsMaxWindow {
		return DegradedAccountsMaxWindow
	}
	return window
}

// NormalizeModelVariant 剥离日期后缀和 -latest 等命名变体。
func NormalizeModelVariant(model string) string {
	normalized := strings.ToLower(strings.TrimSpace(model))
	normalized = modelVariantLatestSuffix.ReplaceAllString(normalized, "")
	normalized = modelVariantISODateSuffix.ReplaceAllString(normalized, "")
	normalized = modelVariantCompactDateSuffix.ReplaceAllString(normalized, "")
	return normalized
}

// ModelDegradationSuspected 判断持久化的模型不匹配记录是否属于真正的降级，而非正常变体。
func ModelDegradationSuspected(sentModel, responseModel string) bool {
	sentModel = strings.TrimSpace(sentModel)
	responseModel = strings.TrimSpace(responseModel)
	if sentModel == "" || responseModel == "" {
		return false
	}
	if upstreamModelsMatchForAudit(sentModel, responseModel) {
		return false
	}
	return NormalizeModelVariant(sentModel) != NormalizeModelVariant(responseModel)
}
