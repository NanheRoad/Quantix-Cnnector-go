package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"quantix-connector-go/internal/store"

	"github.com/gin-gonic/gin"
)

func parseUintParam(c *gin.Context, key string) (uint, bool) {
	raw := strings.TrimSpace(c.Param(key))
	id64, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Invalid ID"})
		return 0, false
	}
	return uint(id64), true
}

func deviceToMap(d store.Device) map[string]any {
	return map[string]any{
		"id":                 d.ID,
		"device_code":        d.DeviceCode,
		"device_category":    d.DeviceCategory,
		"name":               d.Name,
		"protocol_template_id": d.ProtocolTemplateID,
		"connection_params":  store.JSONMapToMap(d.ConnectionParams),
		"template_variables": store.JSONMapToMap(d.TemplateVariables),
		"poll_interval":      d.PollInterval,
		"enabled":            d.Enabled,
		"created_at":         d.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		"updated_at":         d.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
	}
}

func protocolToMap(p store.ProtocolTemplate) map[string]any {
	return map[string]any{
		"id":           p.ID,
		"name":         p.Name,
		"description":  p.Description,
		"protocol_type": p.ProtocolType,
		"template":     store.JSONMapToMap(p.Template),
		"is_system":    p.IsSystem,
		"created_at":   p.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		"updated_at":   p.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
	}
}

func normalizeCode(raw string) (string, error) {
	return store.NormalizeDeviceCode(raw)
}

func build404(detail string) gin.H {
	return gin.H{"detail": detail}
}

func conflictByName(err error, field, fallback string) (int, gin.H) {
	text := strings.ToLower(fmt.Sprintf("%v", err))
	if strings.Contains(text, strings.ToLower(field)) {
		return http.StatusConflict, gin.H{"detail": fallback}
	}
	return http.StatusConflict, gin.H{"detail": fallback}
}
