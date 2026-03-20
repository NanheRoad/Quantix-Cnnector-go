package api

import (
	"context"
	"net/http"
	"strings"

	"quantix-connector-go/internal/store"

	"github.com/gin-gonic/gin"
)

func (s *Server) registerCategoryRoutes(rg *gin.RouterGroup) {
	rg.POST("/api/printers/:device_id/print", s.printerPrint)
	rg.GET("/api/scanners/:device_id/last", s.scannerLast)
	rg.GET("/api/boards/:device_id/status", s.boardStatus)
}

func (s *Server) printerPrint(c *gin.Context) {
	id, ok := parseUintParam(c, "device_id")
	if !ok {
		return
	}
	device, err := s.getDeviceByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Device not found"})
		return
	}
	if strings.TrimSpace(device.DeviceCategory) != "printer_tsc" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Device category mismatch: expected=printer_tsc, actual=" + device.DeviceCategory})
		return
	}
	if !device.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Device is disabled"})
		return
	}
	var req PrintRequest
	_ = c.ShouldBindJSON(&req)
	stepID := ""
	if req.StepID != nil {
		stepID = strings.TrimSpace(*req.StepID)
	}
	if stepID == "" {
		stepID = s.findPrinterManualStep(device.ProtocolTemplateID)
	}
	if stepID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "No printable manual step found in template"})
		return
	}
	result, err := s.manager.ExecuteManualStep(context.Background(), device.ID, stepID, req.Params)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not manual") {
			c.JSON(http.StatusForbidden, gin.H{"detail": err.Error()})
			return
		}
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"detail": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":         true,
		"device_id":  device.ID,
		"device_code": device.DeviceCode,
		"step_id":    stepID,
		"result":     result,
	})
}

func (s *Server) scannerLast(c *gin.Context) {
	id, ok := parseUintParam(c, "device_id")
	if !ok {
		return
	}
	device, err := s.getDeviceByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Device not found"})
		return
	}
	if strings.TrimSpace(device.DeviceCategory) != "scanner" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Device category mismatch: expected=scanner, actual=" + device.DeviceCategory})
		return
	}
	runtime := s.manager.RuntimeSnapshot(device.ID)
	payload, _ := runtime["payload"].(map[string]any)
	if payload == nil {
		payload = map[string]any{}
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":               true,
		"device_id":        device.ID,
		"device_code":      device.DeviceCode,
		"status":           runtime["status"],
		"timestamp":        runtime["timestamp"],
		"error":            runtime["error"],
		"barcode":          payload["barcode"],
		"symbology":        payload["symbology"],
		"deduped":          payload["deduped"],
		"dedupe_window_ms": payload["dedupe_window_ms"],
	})
}

func (s *Server) boardStatus(c *gin.Context) {
	id, ok := parseUintParam(c, "device_id")
	if !ok {
		return
	}
	device, err := s.getDeviceByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Device not found"})
		return
	}
	if strings.TrimSpace(device.DeviceCategory) != "serial_board" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Device category mismatch: expected=serial_board, actual=" + device.DeviceCategory})
		return
	}
	runtime := s.manager.RuntimeSnapshot(device.ID)
	payload, _ := runtime["payload"].(map[string]any)
	if payload == nil {
		payload = map[string]any{}
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":          true,
		"device_id":   device.ID,
		"device_code": device.DeviceCode,
		"status":      runtime["status"],
		"timestamp":   runtime["timestamp"],
		"error":       runtime["error"],
		"board_value": payload["board_value"],
		"board_status": payload["board_status"],
		"alarm":       payload["alarm"],
	})
}

func (s *Server) findPrinterManualStep(protocolTemplateID uint) string {
	var tmpl store.ProtocolTemplate
	if err := s.db.First(&tmpl, protocolTemplateID).Error; err != nil {
		return ""
	}
	template := store.JSONMapToMap(tmpl.Template)
	steps, _ := template["steps"].([]any)
	for _, raw := range steps {
		step, _ := raw.(map[string]any)
		if step == nil {
			continue
		}
		if toStr(valueDefault(step["trigger"], "poll")) != "manual" {
			continue
		}
		action := toStr(step["action"])
		if action == "serial.send" || action == "tcp.send" {
			id := strings.TrimSpace(toStr(step["id"]))
			if id != "" {
				return id
			}
		}
	}
	return ""
}
