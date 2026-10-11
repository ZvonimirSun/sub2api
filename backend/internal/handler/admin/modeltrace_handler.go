package admin

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ModelTraceHandler handles admin operations for ModelTrace model monitoring.
type ModelTraceHandler struct {
	modelTraceSvc *service.ModelTraceService
}

// NewModelTraceHandler creates a new ModelTraceHandler.
func NewModelTraceHandler(modelTraceSvc *service.ModelTraceService) *ModelTraceHandler {
	return &ModelTraceHandler{modelTraceSvc: modelTraceSvc}
}

// ListMonitors GET /admin/modeltrace/monitors
func (h *ModelTraceHandler) ListMonitors(c *gin.Context) {
	monitors, err := h.modelTraceSvc.ListMonitors(c.Request.Context())
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, monitors)
}

// GetGlobalConfig GET /admin/modeltrace/global-config
func (h *ModelTraceHandler) GetGlobalConfig(c *gin.Context) {
	cfg := h.modelTraceSvc.GetGlobalConfig(c.Request.Context())
	c.JSON(http.StatusOK, cfg)
}

// UpdateGlobalConfig PUT /admin/modeltrace/global-config
func (h *ModelTraceHandler) UpdateGlobalConfig(c *gin.Context) {
	var cfg service.ModelTraceGlobalConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.modelTraceSvc.UpdateGlobalConfig(c.Request.Context(), cfg); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "config": h.modelTraceSvc.GetGlobalConfig(c.Request.Context())})
}

// UpdateAccountConfig PUT /admin/modeltrace/accounts/:id
func (h *ModelTraceHandler) UpdateAccountConfig(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid account id")
		return
	}

	var req service.UpdateAccountModelTraceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if err := h.modelTraceSvc.UpdateAccountConfig(c.Request.Context(), accountID, req); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ToggleAccountEnabled PUT /admin/modeltrace/accounts/:id/toggle
func (h *ModelTraceHandler) ToggleAccountEnabled(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid account id")
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if err := h.modelTraceSvc.UpdateAccountEnabled(c.Request.Context(), accountID, req.Enabled); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "enabled": req.Enabled})
}

// ProbeModel POST /admin/modeltrace/accounts/:id/models/:model/run
func (h *ModelTraceHandler) ProbeModel(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid account id")
		return
	}
	modelID := c.Param("model")
	if modelID == "" {
		response.BadRequest(c, "model id required")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()
	record, err := h.modelTraceSvc.ProbeModel(ctx, accountID, modelID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, record)
}
