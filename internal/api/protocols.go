package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"quantix-connector-go/internal/driver"
	"quantix-connector-go/internal/service"
	"quantix-connector-go/internal/store"

	"github.com/gin-gonic/gin"
)

func (s *Server) registerProtocolRoutes(rg *gin.RouterGroup) {
	r := rg.Group("/api/protocols")
	r.GET("", s.listProtocols)
	r.POST("", s.createProtocol)
	r.POST("/import", s.importProtocol)
	r.GET("/:protocol_id", s.getProtocol)
	r.PUT("/:protocol_id", s.updateProtocol)
	r.DELETE("/:protocol_id", s.deleteProtocol)
	r.GET("/:protocol_id/export", s.exportProtocol)
	r.POST("/:protocol_id/test", s.testProtocol)
	r.POST("/:protocol_id/test-step", s.testSingleStep)
}

func (s *Server) listProtocols(c *gin.Context) {
	var rows []store.ProtocolTemplate
	if err := s.db.Order("id asc").Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, protocolToMap(row))
	}
	c.JSON(http.StatusOK, out)
}

func (s *Server) createProtocol(c *gin.Context) {
	var req ProtocolTemplateCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	row := store.ProtocolTemplate{
		Name:         req.Name,
		Description:  req.Description,
		ProtocolType: req.ProtocolType,
		Template:     store.ToJSONMap(req.Template),
		IsSystem:     req.IsSystem,
	}
	if err := s.db.Create(&row).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"detail": "Protocol name already exists"})
		return
	}
	c.JSON(http.StatusOK, protocolToMap(row))
}

func (s *Server) importProtocol(c *gin.Context) {
	var req ProtocolTemplateCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	var exists store.ProtocolTemplate
	if err := s.db.Where("name = ?", req.Name).First(&exists).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"detail": "Protocol name already exists"})
		return
	}
	row := store.ProtocolTemplate{
		Name:         req.Name,
		Description:  req.Description,
		ProtocolType: req.ProtocolType,
		Template:     store.ToJSONMap(req.Template),
		IsSystem:     req.IsSystem,
	}
	if err := s.db.Create(&row).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"detail": "Protocol name already exists"})
		return
	}
	c.JSON(http.StatusOK, protocolToMap(row))
}

func (s *Server) getProtocol(c *gin.Context) {
	id, ok := parseUintParam(c, "protocol_id")
	if !ok {
		return
	}
	var row store.ProtocolTemplate
	if err := s.db.First(&row, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Protocol not found"})
		return
	}
	c.JSON(http.StatusOK, protocolToMap(row))
}

func (s *Server) updateProtocol(c *gin.Context) {
	id, ok := parseUintParam(c, "protocol_id")
	if !ok {
		return
	}
	var row store.ProtocolTemplate
	if err := s.db.First(&row, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Protocol not found"})
		return
	}
	var inUse int64
	_ = s.db.Model(&store.Device{}).Where("protocol_template_id = ?", id).Count(&inUse).Error
	if inUse > 0 {
		c.JSON(http.StatusConflict, gin.H{"detail": "Protocol template is referenced by existing devices and cannot be modified or deleted"})
		return
	}
	var req ProtocolTemplateUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if req.Name != nil {
		row.Name = *req.Name
	}
	if req.Description != nil {
		row.Description = *req.Description
	}
	if req.ProtocolType != nil {
		row.ProtocolType = *req.ProtocolType
	}
	if req.Template != nil {
		row.Template = store.ToJSONMap(req.Template)
	}
	if err := s.db.Save(&row).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, protocolToMap(row))
}

func (s *Server) deleteProtocol(c *gin.Context) {
	id, ok := parseUintParam(c, "protocol_id")
	if !ok {
		return
	}
	var row store.ProtocolTemplate
	if err := s.db.First(&row, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Protocol not found"})
		return
	}
	var inUse int64
	_ = s.db.Model(&store.Device{}).Where("protocol_template_id = ?", id).Count(&inUse).Error
	if inUse > 0 {
		c.JSON(http.StatusConflict, gin.H{"detail": "Protocol template is referenced by existing devices and cannot be modified or deleted"})
		return
	}
	if row.IsSystem {
		c.JSON(http.StatusForbidden, gin.H{"detail": "System protocol can not be deleted"})
		return
	}
	_ = s.db.Delete(&store.ProtocolTemplate{}, id).Error
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) exportProtocol(c *gin.Context) {
	id, ok := parseUintParam(c, "protocol_id")
	if !ok {
		return
	}
	var row store.ProtocolTemplate
	if err := s.db.First(&row, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Protocol not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"name":          row.Name,
		"description":   row.Description,
		"protocol_type": row.ProtocolType,
		"template":      store.JSONMapToMap(row.Template),
	})
}

func (s *Server) testProtocol(c *gin.Context) {
	id, ok := parseUintParam(c, "protocol_id")
	if !ok {
		return
	}
	var row store.ProtocolTemplate
	if err := s.db.First(&row, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Protocol not found"})
		return
	}
	var req ProtocolTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	drv, err := driver.Build(row.ProtocolType, req.ConnectionParams, s.cfg)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	defer drv.Disconnect(context.Background())
	if connected, _ := drv.Connect(context.Background()); !connected {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": "connect failed"})
		return
	}
	exec := service.NewProtocolExecutor()
	template := store.JSONMapToMap(row.Template)
	setup, err := exec.RunSetupSteps(context.Background(), template, drv, req.TemplateVariables)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	steps := setup
	var output map[string]any
	if strings.ToLower(row.ProtocolType) != "mqtt" {
		steps, err = exec.RunPollSteps(context.Background(), template, drv, req.TemplateVariables, steps)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
			return
		}
		ctxMap := map[string]any{"steps": steps}
		for k, v := range req.TemplateVariables {
			ctxMap[k] = v
		}
		output = exec.RenderOutput(template, ctxMap)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "steps": steps, "output": output})
}

func (s *Server) testSingleStep(c *gin.Context) {
	id, ok := parseUintParam(c, "protocol_id")
	if !ok {
		return
	}
	var row store.ProtocolTemplate
	if err := s.db.First(&row, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Protocol not found"})
		return
	}
	var req StepTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	template := store.JSONMapToMap(row.Template)
	step := findStep(template, req.StepID, req.StepContext)
	if step == nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Step not found: " + req.StepID})
		return
	}
	action := toStr(step["action"])
	if !req.AllowWrite && isWriteAction(action) {
		c.JSON(http.StatusOK, gin.H{
			"ok":             false,
			"error":          "写操作需要显式设置 allow_write=true",
			"action":         action,
			"safety_warning": "该操作可能修改设备状态",
		})
		return
	}
	drv, err := driver.Build(row.ProtocolType, req.ConnectionParams, s.cfg)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	defer drv.Disconnect(context.Background())
	if connected, _ := drv.Connect(context.Background()); !connected {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": "connect failed"})
		return
	}
	exec := service.NewProtocolExecutor()
	prev := map[string]any{}
	for k, v := range req.PreviousSteps {
		prev[k] = v
	}
	ctxMap := map[string]any{"steps": prev}
	for k, v := range req.TemplateVariables {
		ctxMap[k] = v
	}
	var stepResult any
	var rendered map[string]any
	if req.StepContext == "event" {
		payload := ""
		if req.TestPayload != nil {
			payload = *req.TestPayload
		}
		ctxMap["payload"] = payload
		stepResult, err = exec.ExecuteOneStep(context.Background(), drv, step, ctxMap, nil, true)
		if err == nil {
			ctxMap["message_handler"] = map[string]any{"result": stepResult}
			rendered = exec.RenderOutput(template, ctxMap)
		}
	} else {
		stepResult, err = exec.ExecuteOneStep(context.Background(), drv, step, ctxMap, nil, false)
		if err == nil {
			merged := map[string]any{}
			for k, v := range prev {
				merged[k] = v
			}
			merged[toStr(step["id"])] = map[string]any{"result": stepResult}
			ctxMap["steps"] = merged
			rendered = exec.RenderOutput(template, ctxMap)
		}
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":              true,
		"step_id":         req.StepID,
		"action":          action,
		"step_result":     stepResult,
		"rendered_output": rendered,
	})
}

func findStep(template map[string]any, stepID, ctx string) map[string]any {
	switch ctx {
	case "setup":
		steps, _ := template["setup_steps"].([]any)
		for _, raw := range steps {
			step, _ := raw.(map[string]any)
			if step != nil && toStr(step["id"]) == stepID {
				return step
			}
		}
		return nil
	case "event":
		step, _ := template["message_handler"].(map[string]any)
		if step != nil && toStr(step["id"]) == stepID {
			return step
		}
		return nil
	default:
		steps, _ := template["steps"].([]any)
		for _, raw := range steps {
			step, _ := raw.(map[string]any)
			if step == nil {
				continue
			}
			if toStr(step["id"]) == stepID && toStr(valueDefault(step["trigger"], "poll")) == "poll" {
				return step
			}
		}
		return nil
	}
}

func isWriteAction(action string) bool {
	switch strings.TrimSpace(strings.ToLower(action)) {
	case "modbus.write_register", "modbus.write_coil", "modbus.write_registers", "modbus.write_coils", "mqtt.publish", "serial.send", "tcp.send":
		return true
	default:
		return false
	}
}

func toStr(v any) string {
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}

func valueDefault(v, fallback any) any {
	if v == nil {
		return fallback
	}
	return v
}
