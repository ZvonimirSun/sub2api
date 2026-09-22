package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type CodexTurnStateHandler struct {
	turnStateService *service.CodexTurnStateService
}

func NewCodexTurnStateHandler(turnStateService *service.CodexTurnStateService) *CodexTurnStateHandler {
	return &CodexTurnStateHandler{
		turnStateService: turnStateService,
	}
}

// GetState 获取所有受监控账号和模型的状态快照。
func (h *CodexTurnStateHandler) GetState(c *gin.Context) {
	if h.turnStateService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Codex turn-state service unavailable")
		return
	}
	overview, err := h.turnStateService.GetState(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Failed to fetch turn-state overview: "+err.Error())
		return
	}
	response.Success(c, overview)
}

type AddCodexMonitoredModelReq struct {
	AccountID             int64  `json:"account_id" binding:"required"`
	Model                 string `json:"model" binding:"required"`
	TargetStateLen        int    `json:"target_state_len"`
	RefreshAdvanceMinutes int    `json:"refresh_advance_minutes"`
	CookieRenewalSeconds  int    `json:"cookie_renewal_seconds"`
}

// AddMonitoredModel 添加受监控模型。
func (h *CodexTurnStateHandler) AddMonitoredModel(c *gin.Context) {
	if h.turnStateService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Codex turn-state service unavailable")
		return
	}
	var req AddCodexMonitoredModelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request payload: "+err.Error())
		return
	}
	if err := h.turnStateService.AddMonitoredModel(
		c.Request.Context(),
		req.AccountID,
		req.Model,
		req.TargetStateLen,
		req.RefreshAdvanceMinutes,
		req.CookieRenewalSeconds,
	); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Model added successfully"})
}

// RemoveMonitoredModel 移除指定账号下的指定监控模型。
func (h *CodexTurnStateHandler) RemoveMonitoredModel(c *gin.Context) {
	if h.turnStateService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Codex turn-state service unavailable")
		return
	}
	idStr := c.Param("id")
	accountID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	model := strings.TrimSpace(c.Param("model"))
	if model == "" {
		response.BadRequest(c, "Invalid model name")
		return
	}

	if err := h.turnStateService.RemoveMonitoredModel(c.Request.Context(), accountID, model); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Model removed successfully"})
}

type ProbeCodexTurnStateReq struct {
	AccountID int64  `json:"account_id" binding:"required"`
	Model     string `json:"model" binding:"required"`
	Force     bool   `json:"force"`
}

// Probe 手动触发一次探针刷新。
func (h *CodexTurnStateHandler) Probe(c *gin.Context) {
	if h.turnStateService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Codex turn-state service unavailable")
		return
	}
	var req ProbeCodexTurnStateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request payload: "+err.Error())
		return
	}

	result, err := h.turnStateService.ProbeAccountModel(c.Request.Context(), req.AccountID, req.Model, req.Force)
	if err != nil {
		msg := err.Error()
		if result != nil && result.ErrorMessage != "" && !strings.Contains(msg, result.ErrorMessage) {
			msg = fmt.Sprintf("%s: %s", msg, result.ErrorMessage)
		}
		response.BadRequest(c, msg)
		return
	}
	if result == nil || result.StatusCode != http.StatusOK || result.TurnState == "" {
		msg := "Probe failed"
		if result != nil && result.ErrorMessage != "" {
			msg = fmt.Sprintf("Probe failed (status %d): %s", result.StatusCode, result.ErrorMessage)
		}
		response.BadRequest(c, msg)
		return
	}
	response.Success(c, result)
}

// GetStats 获取探针指标统计。
func (h *CodexTurnStateHandler) GetStats(c *gin.Context) {
	if h.turnStateService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Codex turn-state service unavailable")
		return
	}
	stats := h.turnStateService.GetStats(c.Request.Context())
	response.Success(c, stats)
}

// GetHistory 获取探针历史列表。
func (h *CodexTurnStateHandler) GetHistory(c *gin.Context) {
	if h.turnStateService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Codex turn-state service unavailable")
		return
	}
	history := h.turnStateService.GetHistory(c.Request.Context())
	response.Success(c, history)
}

// GetProxies 获取探针专用代理配置（包含静态优先池与动态兜底池）。
func (h *CodexTurnStateHandler) GetProxies(c *gin.Context) {
	if h.turnStateService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Codex turn-state service unavailable")
		return
	}
	cfg, err := h.turnStateService.GetProxyConfig(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Failed to read proxies: "+err.Error())
		return
	}
	response.Success(c, cfg)
}

type SaveProxiesReq struct {
	StaticProxies        []string `json:"static_proxies"`
	DynamicProxies       []string `json:"dynamic_proxies"`
	Proxies              []string `json:"proxies"` // 兼容旧字段
	CookieRenewalSeconds int      `json:"cookie_renewal_seconds"`
}

// SaveProxies 保存探针专用代理配置。
func (h *CodexTurnStateHandler) SaveProxies(c *gin.Context) {
	if h.turnStateService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Codex turn-state service unavailable")
		return
	}
	var req SaveProxiesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg := &service.CodexTurnStateProxyConfig{
		StaticProxies:        req.StaticProxies,
		DynamicProxies:       req.DynamicProxies,
		CookieRenewalSeconds: req.CookieRenewalSeconds,
	}
	if len(cfg.StaticProxies) == 0 && len(req.Proxies) > 0 {
		cfg.StaticProxies = req.Proxies
	}
	if err := h.turnStateService.SetProxyConfig(c.Request.Context(), cfg); err != nil {
		response.InternalError(c, "Failed to save proxies: "+err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Proxies saved successfully"})
}

// GetDegraded 查询近期的模型降级情况。
func (h *CodexTurnStateHandler) GetDegraded(c *gin.Context) {
	if h.turnStateService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Codex turn-state service unavailable")
		return
	}
	window, ok := service.ParseDegradedAccountsWindow(c.Query("window"))
	if !ok {
		response.BadRequest(c, "Invalid window duration")
		return
	}
	degraded, err := h.turnStateService.GetDegraded(c.Request.Context(), window)
	if err != nil {
		response.InternalError(c, "Failed to query degradation: "+err.Error())
		return
	}
	response.Success(c, degraded)
}

// ClearHistory 清理探测历史记录。
func (h *CodexTurnStateHandler) ClearHistory(c *gin.Context) {
	if h.turnStateService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Codex turn-state service unavailable")
		return
	}
	var accIDPtr *int64
	if idStr := strings.TrimSpace(c.Query("account_id")); idStr != "" {
		if id, err := strconv.ParseInt(idStr, 10, 64); err == nil && id > 0 {
			accIDPtr = &id
		}
	}
	model := strings.TrimSpace(c.Query("model"))
	h.turnStateService.ClearHistory(accIDPtr, model)
	response.Success(c, gin.H{"message": "Probe history cleared successfully"})
}

type ToggleMonitoredModelReq struct {
	Enabled bool `json:"enabled"`
}

// ToggleMonitoredModel 启用或暂停某个模型的监控。
func (h *CodexTurnStateHandler) ToggleMonitoredModel(c *gin.Context) {
	if h.turnStateService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Codex turn-state service unavailable")
		return
	}
	idStr := c.Param("id")
	accountID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	model := strings.TrimSpace(c.Param("model"))
	if model == "" {
		response.BadRequest(c, "Invalid model name")
		return
	}
	var req ToggleMonitoredModelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request payload: "+err.Error())
		return
	}
	if err := h.turnStateService.ToggleMonitoredModel(c.Request.Context(), accountID, model, req.Enabled); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Model status updated successfully"})
}
