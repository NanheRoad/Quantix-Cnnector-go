package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"quantix-connector-go/internal/store"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (s *Server) registerDeviceRoutes(rg *gin.RouterGroup) {
	r := rg.Group("/api/devices")
	r.GET("", s.listDevices)
	r.POST("", s.createDevice)
	r.GET("/by-code/:device_code", s.getDeviceByCode)
	r.PUT("/by-code/:device_code", s.updateDeviceByCode)
	r.DELETE("/by-code/:device_code", s.deleteDeviceByCode)
	r.POST("/by-code/:device_code/enable", s.enableDeviceByCode)
	r.POST("/by-code/:device_code/disable", s.disableDeviceByCode)
	r.POST("/by-code/:device_code/execute", s.executeDeviceByCode)
	r.POST("/test-connection", s.testDeviceConnection)

	r.GET("/:device_id", s.getDevice)
	r.PUT("/:device_id", s.updateDevice)
	r.DELETE("/:device_id", s.deleteDevice)
	r.POST("/:device_id/enable", s.enableDevice)
	r.POST("/:device_id/disable", s.disableDevice)
	r.POST("/:device_id/execute", s.executeDevice)
}

func (s *Server) testDeviceConnection(c *gin.Context) {
	var req DeviceConnectionTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	var tmpl store.ProtocolTemplate
	if err := s.db.First(&tmpl, req.ProtocolTemplateID).Error; err != nil {
		c.JSON(http.StatusNotFound, build404("Protocol template not found"))
		return
	}
	protocolType := strings.ToLower(strings.TrimSpace(tmpl.ProtocolType))
	if protocolType == "serial" || protocolType == "modbus_rtu" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "串口类型请使用“串口调试”页面进行连接测试"})
		return
	}
	if protocolType != "modbus_tcp" && protocolType != "tcp" && protocolType != "mqtt" && protocolType != "modbus" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": fmt.Sprintf("不支持该协议类型的连接测试: %s", protocolType)})
		return
	}
	host := strings.TrimSpace(fmt.Sprintf("%v", req.ConnectionParams["host"]))
	if host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "connection_params.host is required"})
		return
	}
	port, ok := parsePort(req.ConnectionParams["port"], defaultPortForProtocol(protocolType))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "connection_params.port is invalid"})
		return
	}
	timeout := time.Duration(req.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 1500 * time.Millisecond
	}
	if timeout > 15*time.Second {
		timeout = 15 * time.Second
	}
	endpoint := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", endpoint, timeout)
	elapsed := float64(time.Since(start).Microseconds()) / 1000.0
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"detail":        fmt.Sprintf("连接失败: %v", err),
			"protocol_type": protocolType,
			"endpoint":      endpoint,
			"elapsed_ms":    elapsed,
		})
		return
	}
	_ = conn.Close()
	c.JSON(http.StatusOK, gin.H{
		"ok":            true,
		"protocol_type": protocolType,
		"endpoint":      endpoint,
		"elapsed_ms":    elapsed,
	})
}

func defaultPortForProtocol(protocolType string) int {
	switch strings.ToLower(strings.TrimSpace(protocolType)) {
	case "mqtt":
		return 1883
	default:
		return 502
	}
}

func parsePort(raw any, fallback int) (int, bool) {
	if raw == nil {
		return fallback, true
	}
	switch t := raw.(type) {
	case int:
		return t, t > 0 && t <= 65535
	case int32:
		v := int(t)
		return v, v > 0 && v <= 65535
	case int64:
		v := int(t)
		return v, v > 0 && v <= 65535
	case uint:
		v := int(t)
		return v, v > 0 && v <= 65535
	case uint32:
		v := int(t)
		return v, v > 0 && v <= 65535
	case uint64:
		v := int(t)
		return v, v > 0 && v <= 65535
	case float32:
		v := int(t)
		return v, v > 0 && v <= 65535
	case float64:
		v := int(t)
		return v, v > 0 && v <= 65535
	case json.Number:
		if n, err := t.Int64(); err == nil {
			v := int(n)
			return v, v > 0 && v <= 65535
		}
		if f, err := t.Float64(); err == nil {
			v := int(f)
			return v, v > 0 && v <= 65535
		}
		return 0, false
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return fallback, true
		}
		if n, err := strconv.Atoi(s); err == nil {
			return n, n > 0 && n <= 65535
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			n := int(f)
			return n, n > 0 && n <= 65535
		}
		return 0, false
	default:
		return 0, false
	}
}

func (s *Server) listDevices(c *gin.Context) {
	var rows []store.Device
	if err := s.db.Order("id asc").Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}
	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	rtMap := s.manager.RuntimeSnapshots(ids)
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := deviceToMap(row)
		runtime, ok := rtMap[row.ID]
		if !ok {
			runtime = map[string]any{
				"type":      "weight_update",
				"status":    "offline",
				"weight":    nil,
				"unit":      "kg",
				"payload":   map[string]any{},
				"timestamp": nil,
				"error":     nil,
			}
		}
		item["runtime"] = runtime
		result = append(result, item)
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) createDevice(c *gin.Context) {
	var req DeviceCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if err := validateDeviceCode(req.DeviceCode); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if err := validateDeviceCategory(req.DeviceCategory); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	var tmpl store.ProtocolTemplate
	if err := s.db.First(&tmpl, req.ProtocolTemplateID).Error; err != nil {
		c.JSON(http.StatusNotFound, build404("Protocol template not found"))
		return
	}
	code, _ := store.NormalizeDeviceCode(req.DeviceCode)
	category, _ := store.NormalizeDeviceCategory(req.DeviceCategory)
	pollInterval := req.PollInterval
	if pollInterval <= 0 {
		pollInterval = 1.0
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	row := store.Device{
		DeviceCode:         code,
		DeviceCategory:     category,
		Name:               req.Name,
		ProtocolTemplateID: req.ProtocolTemplateID,
		ConnectionParams:   store.ToJSONMap(req.ConnectionParams),
		TemplateVariables:  store.ToJSONMap(req.TemplateVariables),
		PollInterval:       pollInterval,
		Enabled:            enabled,
	}
	if err := s.db.Create(&row).Error; err != nil {
		text := strings.ToLower(err.Error())
		if strings.Contains(text, "device_code") {
			c.JSON(http.StatusConflict, gin.H{"detail": "Device code already exists"})
			return
		}
		if strings.Contains(text, "name") {
			c.JSON(http.StatusConflict, gin.H{"detail": "Device name already exists"})
			return
		}
		c.JSON(http.StatusConflict, gin.H{"detail": "Device unique constraint violated"})
		return
	}
	_ = s.manager.ReloadDevice(context.Background(), row.ID)
	c.JSON(http.StatusOK, deviceToMap(row))
}

func (s *Server) getDeviceByCode(c *gin.Context) {
	row, ok := s.getDeviceByCodeInternal(c, c.Param("device_code"))
	if !ok {
		return
	}
	item := deviceToMap(*row)
	item["runtime"] = s.manager.RuntimeSnapshot(row.ID)
	c.JSON(http.StatusOK, item)
}

func (s *Server) updateDeviceByCode(c *gin.Context) {
	row, ok := s.getDeviceByCodeInternal(c, c.Param("device_code"))
	if !ok {
		return
	}
	s.updateDeviceRow(c, row)
}

func (s *Server) deleteDeviceByCode(c *gin.Context) {
	row, ok := s.getDeviceByCodeInternal(c, c.Param("device_code"))
	if !ok {
		return
	}
	_ = s.manager.RemoveDevice(context.Background(), row.ID)
	_ = s.db.Delete(&store.Device{}, row.ID).Error
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) enableDeviceByCode(c *gin.Context) {
	row, ok := s.getDeviceByCodeInternal(c, c.Param("device_code"))
	if !ok {
		return
	}
	row.Enabled = true
	_ = s.db.Save(row).Error
	_ = s.manager.ReloadDevice(context.Background(), row.ID)
	c.JSON(http.StatusOK, deviceToMap(*row))
}

func (s *Server) disableDeviceByCode(c *gin.Context) {
	row, ok := s.getDeviceByCodeInternal(c, c.Param("device_code"))
	if !ok {
		return
	}
	row.Enabled = false
	_ = s.db.Save(row).Error
	_ = s.manager.ReloadDevice(context.Background(), row.ID)
	c.JSON(http.StatusOK, deviceToMap(*row))
}

func (s *Server) executeDeviceByCode(c *gin.Context) {
	row, ok := s.getDeviceByCodeInternal(c, c.Param("device_code"))
	if !ok {
		return
	}
	s.executeDeviceRow(c, row.ID, row.Enabled)
}

func (s *Server) getDevice(c *gin.Context) {
	id, ok := parseUintParam(c, "device_id")
	if !ok {
		return
	}
	row, err := s.getDeviceByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, build404("Device not found"))
		return
	}
	item := deviceToMap(*row)
	item["runtime"] = s.manager.RuntimeSnapshot(row.ID)
	c.JSON(http.StatusOK, item)
}

func (s *Server) updateDevice(c *gin.Context) {
	id, ok := parseUintParam(c, "device_id")
	if !ok {
		return
	}
	row, err := s.getDeviceByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, build404("Device not found"))
		return
	}
	s.updateDeviceRow(c, row)
}

func (s *Server) deleteDevice(c *gin.Context) {
	id, ok := parseUintParam(c, "device_id")
	if !ok {
		return
	}
	row, err := s.getDeviceByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, build404("Device not found"))
		return
	}
	_ = s.manager.RemoveDevice(context.Background(), row.ID)
	_ = s.db.Delete(&store.Device{}, row.ID).Error
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) enableDevice(c *gin.Context) {
	id, ok := parseUintParam(c, "device_id")
	if !ok {
		return
	}
	row, err := s.getDeviceByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, build404("Device not found"))
		return
	}
	row.Enabled = true
	_ = s.db.Save(row).Error
	_ = s.manager.ReloadDevice(context.Background(), row.ID)
	c.JSON(http.StatusOK, deviceToMap(*row))
}

func (s *Server) disableDevice(c *gin.Context) {
	id, ok := parseUintParam(c, "device_id")
	if !ok {
		return
	}
	row, err := s.getDeviceByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, build404("Device not found"))
		return
	}
	row.Enabled = false
	_ = s.db.Save(row).Error
	_ = s.manager.ReloadDevice(context.Background(), row.ID)
	c.JSON(http.StatusOK, deviceToMap(*row))
}

func (s *Server) executeDevice(c *gin.Context) {
	id, ok := parseUintParam(c, "device_id")
	if !ok {
		return
	}
	row, err := s.getDeviceByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, build404("Device not found"))
		return
	}
	s.executeDeviceRow(c, id, row.Enabled)
}

func (s *Server) executeDeviceRow(c *gin.Context, deviceID uint, enabled bool) {
	var req ExecuteStepRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if !enabled {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Device is disabled"})
		return
	}
	result, err := s.manager.ExecuteManualStep(context.Background(), deviceID, req.StepID, req.Params)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "not manual") {
			c.JSON(http.StatusForbidden, gin.H{"detail": msg})
			return
		}
		if strings.Contains(strings.ToLower(msg), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"detail": msg})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"detail": msg})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) updateDeviceRow(c *gin.Context, row *store.Device) {
	var req DeviceUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if req.DeviceCode != nil {
		if err := validateDeviceCode(*req.DeviceCode); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
			return
		}
		code, _ := store.NormalizeDeviceCode(*req.DeviceCode)
		row.DeviceCode = code
	}
	if req.DeviceCategory != nil {
		if err := validateDeviceCategory(*req.DeviceCategory); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
			return
		}
		cat, _ := store.NormalizeDeviceCategory(*req.DeviceCategory)
		row.DeviceCategory = cat
	}
	if req.Name != nil {
		row.Name = *req.Name
	}
	if req.ProtocolTemplateID != nil {
		var tmpl store.ProtocolTemplate
		if err := s.db.First(&tmpl, *req.ProtocolTemplateID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"detail": "Protocol template not found"})
			return
		}
		row.ProtocolTemplateID = *req.ProtocolTemplateID
	}
	if req.ConnectionParams != nil {
		row.ConnectionParams = store.ToJSONMap(req.ConnectionParams)
	}
	if req.TemplateVariables != nil {
		row.TemplateVariables = store.ToJSONMap(req.TemplateVariables)
	}
	if req.PollInterval != nil {
		row.PollInterval = *req.PollInterval
	}
	if req.Enabled != nil {
		row.Enabled = *req.Enabled
	}
	if err := s.db.Save(row).Error; err != nil {
		text := strings.ToLower(err.Error())
		if strings.Contains(text, "device_code") {
			c.JSON(http.StatusConflict, gin.H{"detail": "Device code already exists"})
			return
		}
		if strings.Contains(text, "name") {
			c.JSON(http.StatusConflict, gin.H{"detail": "Device name already exists"})
			return
		}
		c.JSON(http.StatusConflict, gin.H{"detail": "Device unique constraint violated"})
		return
	}
	_ = s.manager.ReloadDevice(context.Background(), row.ID)
	c.JSON(http.StatusOK, deviceToMap(*row))
}

func (s *Server) getDeviceByID(id uint) (*store.Device, error) {
	var row store.Device
	err := s.db.First(&row, id).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Server) getDeviceByCodeInternal(c *gin.Context, code string) (*store.Device, bool) {
	normalized, err := normalizeCode(code)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return nil, false
	}
	var row store.Device
	err = s.db.Where("device_code = ?", normalized).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, build404("Device not found"))
		return nil, false
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": fmt.Sprintf("%v", err)})
		return nil, false
	}
	return &row, true
}
