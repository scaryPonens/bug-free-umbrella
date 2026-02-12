package handler

import (
	"net/http"
	"strconv"
	"strings"

	"bug-free-umbrella/internal/domain"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
)

// GetSignals godoc
// @Summary      Get generated trading signals
// @Description  Returns recent signals, optionally filtered by symbol/risk/indicator
// @Tags         signals
// @Produce      json
// @Param        symbol     query  string  false  "Asset symbol (e.g., BTC, ETH)"
// @Param        risk       query  int     false  "Risk level (1-5)"
// @Param        indicator  query  string  false  "Indicator key (rsi, macd, bollinger, volume_zscore)"
// @Param        limit      query  int     false  "Number of signals (default 50, max 200)"  default(50)
// @Success      200  {object}  map[string]interface{}
// @Failure      400  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Router       /api/signals [get]
func (h *Handler) GetSignals(c *gin.Context) {
	if h.signalService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "signal service unavailable"})
		return
	}

	ctx, span := h.tracer.Start(c.Request.Context(), "handler.get-signals")
	defer span.End()

	filter := domain.SignalFilter{
		Symbol:    strings.ToUpper(strings.TrimSpace(c.Query("symbol"))),
		Indicator: strings.ToLower(strings.TrimSpace(c.Query("indicator"))),
	}

	if filter.Symbol != "" {
		span.SetAttributes(attribute.String("symbol", filter.Symbol))
		if _, ok := domain.CoinGeckoID[filter.Symbol]; !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "unsupported symbol: " + filter.Symbol,
				"supported_symbols": domain.SupportedSymbols,
			})
			return
		}
	}

	if rawRisk := strings.TrimSpace(c.Query("risk")); rawRisk != "" {
		r, err := strconv.Atoi(rawRisk)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "risk must be an integer between 1 and 5"})
			return
		}
		risk := domain.RiskLevel(r)
		if !risk.IsValid() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "risk must be between 1 and 5"})
			return
		}
		filter.Risk = &risk
	}

	limit := 50
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		n, err := strconv.Atoi(rawLimit)
		if err != nil || n <= 0 || n > 200 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 200"})
			return
		}
		limit = n
	}
	filter.Limit = limit

	signals, err := h.signalService.ListSignals(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"signals": signals})
}
