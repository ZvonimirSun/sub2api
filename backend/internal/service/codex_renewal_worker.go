package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	DefaultCodexRenewalInterval = 10 * time.Second
)

// CodexRenewalWorker 是常驻后台的 Turn-State 自动轮巡探针与续期工作组件。
type CodexRenewalWorker struct {
	service  *CodexTurnStateService
	interval time.Duration
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func NewCodexRenewalWorker(service *CodexTurnStateService, interval time.Duration) *CodexRenewalWorker {
	if interval <= 0 {
		interval = DefaultCodexRenewalInterval
	}
	return &CodexRenewalWorker{
		service:  service,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start 启动后台轮巡任务。
func (w *CodexRenewalWorker) Start() {
	if w == nil || w.service == nil {
		return
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()

		// 启动后稍作延迟再执行首次检查，避免与系统启动争抢资源
		time.Sleep(3 * time.Second)
		w.runCycle()

		for {
			select {
			case <-ticker.C:
				w.runCycle()
			case <-w.stopCh:
				return
			}
		}
	}()
}

// Stop 停止后台任务并等待当前周期退出。
func (w *CodexRenewalWorker) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() {
		close(w.stopCh)
	})
	w.wg.Wait()
}

func (w *CodexRenewalWorker) runCycle() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	overview, err := w.service.GetState(ctx)
	if err != nil {
		logger.LegacyPrintf("service.codex_renewal", "failed to fetch state overview: %v", err)
		return
	}

	if overview.StaticProxyCount == 0 && overview.DynamicProxyCount == 0 {
		// 未配置探针代理池，跳过本次自动轮巡
		return
	}

	for _, acc := range overview.Accounts {
		// 单账号下的模型严格串行探测
		for _, modelSlot := range acc.Models {
			select {
			case <-w.stopCh:
				return
			case <-ctx.Done():
				return
			default:
			}

			if !modelSlot.Enabled {
				continue
			}

			// 如果当前槽位处于退避冷却中，跳过
			if modelSlot.InCooldown {
				continue
			}

			needsProbe := false
			reason := ""

			insp := modelSlot.StateInspection
			cookieRenewalSec := modelSlot.CookieRenewalSeconds
			if cookieRenewalSec <= 0 {
				cookieRenewalSec = DefaultCodexCookieRenewalAge
			}

			if insp == nil || !insp.Valid {
				needsProbe = true
				reason = "missing_or_invalid"
			} else if insp.IsExpired {
				needsProbe = true
				reason = "expired"
			} else if insp.RemainingSeconds <= modelSlot.RefreshAdvanceMinutes*60 {
				needsProbe = true
				reason = "expiring_soon"
			} else if modelSlot.DegradationStatus == "degraded" {
				needsProbe = true
				reason = "degraded_detected"
			} else if modelSlot.Cflb == "" || modelSlot.Oailb == "" {
				// 缺少 __cflb 或 __oailb 负载均衡 Cookie，立即触发探测获取
				needsProbe = true
				reason = "missing_lb_cookies"
			} else if modelSlot.CookieAgeSeconds >= int64(cookieRenewalSec) {
				// Cookie 年龄到达配置的续期阈值（默认 150s），自动触发新探测续期
				needsProbe = true
				reason = fmt.Sprintf("cookie_renewal_due_%ds", cookieRenewalSec)
			}

			if !needsProbe {
				continue
			}

			logger.LegacyPrintf("service.codex_renewal", "starting probe: account_id=%d model=%s reason=%s", acc.AccountID, modelSlot.Model, reason)

			res, probeErr := w.service.ProbeAccountModel(ctx, acc.AccountID, modelSlot.Model, false)
			if probeErr != nil {
				logger.LegacyPrintf("service.codex_renewal", "probe failed: account_id=%d model=%s err=%v", acc.AccountID, modelSlot.Model, probeErr)
			} else if res != nil && res.StatusCode == 200 {
				logger.LegacyPrintf("service.codex_renewal", "probe succeeded: account_id=%d model=%s state_len=%d", acc.AccountID, modelSlot.Model, len(res.TurnState))
			}

			// 槽位之间休眠 500ms，平滑上游调用
			time.Sleep(500 * time.Millisecond)
		}
	}
}
