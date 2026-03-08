package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// GetBacktestSummary godoc
// @Summary      Get backtest accuracy summary
// @Description  Returns all-time ML accuracy summary by model key
// @Tags         backtest
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Failure      503  {object}  map[string]string
// @Security     ApiKeyAuth
// @Router       /api/backtest/summary [get]
func (h *Handler) GetBacktestSummary(c *gin.Context) {
	if h.backtestService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest service unavailable"})
		return
	}
	ctx, span := h.tracer.Start(c.Request.Context(), "handler.get-backtest-summary")
	defer span.End()

	summary, err := h.backtestService.GetSummary(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"summary": summary})
}

// GetBacktestDaily godoc
// @Summary      Get backtest daily accuracy
// @Description  Returns per-day ML accuracy for an optional model key
// @Tags         backtest
// @Produce      json
// @Param        model  query  string  false  "Model key"
// @Param        days   query  int     false  "Days of history" default(30)
// @Success      200  {object}  map[string]interface{}
// @Failure      503  {object}  map[string]string
// @Security     ApiKeyAuth
// @Router       /api/backtest/daily [get]
func (h *Handler) GetBacktestDaily(c *gin.Context) {
	if h.backtestService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest service unavailable"})
		return
	}
	ctx, span := h.tracer.Start(c.Request.Context(), "handler.get-backtest-daily")
	defer span.End()

	model := strings.TrimSpace(c.Query("model"))
	days := 30
	if rawDays := strings.TrimSpace(c.Query("days")); rawDays != "" {
		n, err := strconv.Atoi(rawDays)
		if err != nil || n <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "days must be a positive integer"})
			return
		}
		days = n
	}

	daily, err := h.backtestService.GetDaily(ctx, model, days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"daily": daily})
}

// GetBacktestPredictions godoc
// @Summary      Get recent resolved ML predictions
// @Description  Returns recent resolved ML predictions used for backtest view
// @Tags         backtest
// @Produce      json
// @Param        limit  query  int  false  "number of predictions" default(50)
// @Success      200  {object}  map[string]interface{}
// @Failure      503  {object}  map[string]string
// @Security     ApiKeyAuth
// @Router       /api/backtest/predictions [get]
func (h *Handler) GetBacktestPredictions(c *gin.Context) {
	if h.backtestService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtest service unavailable"})
		return
	}
	ctx, span := h.tracer.Start(c.Request.Context(), "handler.get-backtest-predictions")
	defer span.End()

	limit := 50
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		n, err := strconv.Atoi(rawLimit)
		if err != nil || n <= 0 || n > 200 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 200"})
			return
		}
		limit = n
	}

	preds, err := h.backtestService.GetPredictions(ctx, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"predictions": preds})
}
