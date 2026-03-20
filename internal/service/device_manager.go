package service

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"quantix-connector-go/internal/config"
	"quantix-connector-go/internal/driver"
	"quantix-connector-go/internal/store"

	"gorm.io/gorm"
)

type deviceRuntime struct {
	device       store.Device
	template     store.ProtocolTemplate
	driver       driver.Driver
	state        *RuntimeState
	stopCh       chan struct{}
	doneCh       chan struct{}
	ioMu         sync.Mutex
	dedupeMu     sync.Mutex
	lastScanCode string
	lastScanAt   time.Time
}

type DeviceManager struct {
	db       *gorm.DB
	cfg      config.Settings
	executor *ProtocolExecutor
	bus      *EventBus
	metrics  *RuntimeMetrics
	mu       sync.Mutex
	runtimes map[uint]*deviceRuntime
}

func NewDeviceManager(db *gorm.DB, cfg config.Settings) *DeviceManager {
	return &DeviceManager{
		db:       db,
		cfg:      cfg,
		executor: NewProtocolExecutor(),
		bus:      NewEventBus(200),
		metrics:  NewRuntimeMetrics(4096),
		runtimes: map[uint]*deviceRuntime{},
	}
}

func (m *DeviceManager) Startup(ctx context.Context) error {
	var devices []store.Device
	if err := m.db.Preload("ProtocolTemplate").Where("enabled = ?", true).Find(&devices).Error; err != nil {
		return err
	}
	var firstErr error
	for i := range devices {
		if err := m.StartDevice(ctx, devices[i].ID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *DeviceManager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	ids := make([]uint, 0, len(m.runtimes))
	for id := range m.runtimes {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		_ = m.StopDevice(ctx, id)
	}
	return nil
}

func (m *DeviceManager) StartDevice(ctx context.Context, deviceID uint) error {
	var device store.Device
	if err := m.db.First(&device, deviceID).Error; err != nil {
		return err
	}
	var tmpl store.ProtocolTemplate
	if err := m.db.First(&tmpl, device.ProtocolTemplateID).Error; err != nil {
		return fmt.Errorf("missing protocol template for device_id=%d", deviceID)
	}
	_ = m.StopDevice(ctx, deviceID)
	drv, err := driver.Build(tmpl.ProtocolType, store.JSONMapToMap(device.ConnectionParams), m.cfg)
	if err != nil {
		return err
	}
	rt := &deviceRuntime{
		device:   device,
		template: tmpl,
		driver:   drv,
		state:    NewRuntimeState(device.ID, device.Name, device.DeviceCode, normalizeCategory(device.DeviceCategory)),
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
	if _, ok := drv.(*driver.MqttDriver); ok {
		drv.RegisterMessageHandler(func(topic string, payload []byte) {
			m.handleMqttMessage(rt, topic, payload)
		})
	}
	m.mu.Lock()
	m.runtimes[device.ID] = rt
	m.mu.Unlock()
	go m.superviseRuntime(rt)
	return nil
}

func (m *DeviceManager) StopDevice(ctx context.Context, deviceID uint) error {
	m.mu.Lock()
	rt := m.runtimes[deviceID]
	if rt != nil {
		delete(m.runtimes, deviceID)
	}
	m.mu.Unlock()
	if rt == nil {
		return nil
	}
	close(rt.stopCh)
	select {
	case <-rt.doneCh:
	case <-time.After(2 * time.Second):
	}
	_ = rt.driver.Disconnect(ctx)
	rt.ioMu.Lock()
	rt.state.MarkOffline("stopped")
	msg := rt.state.ToMessage()
	rt.ioMu.Unlock()
	m.publishEvent(msg)
	return nil
}

func (m *DeviceManager) ReloadDevice(ctx context.Context, deviceID uint) error {
	var d store.Device
	if err := m.db.First(&d, deviceID).Error; err != nil {
		return m.StopDevice(ctx, deviceID)
	}
	if !d.Enabled {
		return m.StopDevice(ctx, deviceID)
	}
	return m.StartDevice(ctx, deviceID)
}

func (m *DeviceManager) RemoveDevice(ctx context.Context, deviceID uint) error {
	return m.StopDevice(ctx, deviceID)
}

func (m *DeviceManager) ExecuteManualStep(ctx context.Context, deviceID uint, stepID string, paramsOverride map[string]any) (map[string]any, error) {
	rt := m.GetRuntime(deviceID)
	if rt == nil {
		return nil, fmt.Errorf("Device runtime not found or not enabled")
	}
	rt.ioMu.Lock()
	defer rt.ioMu.Unlock()
	vars := store.JSONMapToMap(rt.device.TemplateVariables)
	result, err := m.executor.RunManualStep(ctx, store.JSONMapToMap(rt.template.Template), rt.driver, stepID, vars, paramsOverride, rt.state.StepResults)
	if err != nil {
		return nil, err
	}
	rt.state.StepResults[stepID] = map[string]any{"result": result["result"]}
	output, _ := result["output"].(map[string]any)
	weight := toFloatPtr(mgrValueOr(output["weight"], rt.state.Weight))
	unit := normalizeUnit(fmt.Sprintf("%v", mgrValueOr(output["unit"], rt.state.Unit)))
	category := normalizeCategory(rt.device.DeviceCategory)
	eventType := eventTypeForCategory(category)
	payload := buildRuntimePayload(output)
	rt.state.MarkOnline(weight, unit, eventType, payload)
	m.publishEvent(rt.state.ToMessage())
	return result, nil
}

func (m *DeviceManager) Subscribe() chan map[string]any {
	return m.bus.Subscribe()
}

func (m *DeviceManager) Unsubscribe(ch chan map[string]any) {
	m.bus.Unsubscribe(ch)
}

func (m *DeviceManager) GetRuntime(deviceID uint) *deviceRuntime {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runtimes[deviceID]
}

func (m *DeviceManager) RuntimeSnapshot(deviceID uint) map[string]any {
	rt := m.GetRuntime(deviceID)
	if rt == nil {
		return offlineRuntimeMessage()
	}
	rt.ioMu.Lock()
	defer rt.ioMu.Unlock()
	return rt.state.ToMessage()
}

func (m *DeviceManager) RuntimeSnapshots(deviceIDs []uint) map[uint]map[string]any {
	out := map[uint]map[string]any{}
	for _, id := range deviceIDs {
		out[id] = m.RuntimeSnapshot(id)
	}
	return out
}

func (m *DeviceManager) HealthSnapshot() map[string]any {
	m.mu.Lock()
	entries := make([]*deviceRuntime, 0, len(m.runtimes))
	for _, rt := range m.runtimes {
		entries = append(entries, rt)
	}
	m.mu.Unlock()

	online := 0
	offline := 0
	errs := 0
	for _, rt := range entries {
		rt.ioMu.Lock()
		status := rt.state.Status
		rt.ioMu.Unlock()
		switch status {
		case "online":
			online++
		case "error":
			errs++
		default:
			offline++
		}
	}

	busStats := m.bus.Stats()
	status := "ok"
	if errs > 0 {
		status = "degraded"
	}
	if dropped, ok := busStats["dropped"].(uint64); ok && dropped > 0 {
		status = "degraded"
	}
	return map[string]any{
		"status":        status,
		"runtime_count": len(entries),
		"online_count":  online,
		"offline_count": offline,
		"error_count":   errs,
		"metrics":       m.metrics.Snapshot(busStats),
		"timestamp":     time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func (m *DeviceManager) publishEvent(message map[string]any) {
	m.metrics.IncPublishedEvent()
	m.bus.Publish(message)
}

func (m *DeviceManager) superviseRuntime(rt *deviceRuntime) {
	defer close(rt.doneCh)
	for {
		panicked := false
		func() {
			defer func() {
				if r := recover(); r != nil {
					panicked = true
					errText := fmt.Sprintf("runtime panic: %v; stack=%s", r, strings.TrimSpace(string(debug.Stack())))
					m.publishEvent(map[string]any{
						"type":            eventTypeForCategory(normalizeCategory(rt.device.DeviceCategory)),
						"device_id":       rt.device.ID,
						"device_name":     rt.device.Name,
						"device_code":     rt.device.DeviceCode,
						"device_category": normalizeCategory(rt.device.DeviceCategory),
						"status":          "error",
						"weight":          nil,
						"unit":            "kg",
						"payload":         map[string]any{},
						"timestamp":       time.Now().UTC().Format(time.RFC3339Nano),
						"error":           errText,
					})
				}
			}()
			m.runRuntimeLoop(rt)
		}()
		if !panicked {
			return
		}
		m.metrics.IncRuntimeRestart()
		if waitOrStop(rt.stopCh, 200*time.Millisecond) {
			return
		}
	}
}

func (m *DeviceManager) runRuntimeLoop(rt *deviceRuntime) {
	backoff := time.Second
	setupDone := false
	for {
		select {
		case <-rt.stopCh:
			return
		default:
		}

		cycleStart := time.Now()
		stage := "connect"
		if !rt.driver.IsConnected() {
			rt.ioMu.Lock()
			connected, _ := rt.driver.Connect(context.Background())
			rt.ioMu.Unlock()
			if !connected {
				m.metrics.IncReconnect()
				errText := rt.driver.LastError()
				if errText == "" {
					errText = "connect failed"
				}
				rt.ioMu.Lock()
				rt.state.MarkOffline(formatError(stage, rt, "connect_failed", errText))
				msg := rt.state.ToMessage()
				rt.ioMu.Unlock()
				m.publishEvent(msg)
				if waitOrStop(rt.stopCh, backoff) {
					return
				}
				if backoff < 30*time.Second {
					backoff *= 2
				}
				continue
			}
		}
		backoff = time.Second

		if !setupDone {
			stage = "setup"
			rt.ioMu.Lock()
			setup, err := m.executor.RunSetupSteps(context.Background(), store.JSONMapToMap(rt.template.Template), rt.driver, store.JSONMapToMap(rt.device.TemplateVariables))
			if err != nil {
				rt.state.MarkError(formatException(stage, rt, err))
				msg := rt.state.ToMessage()
				rt.ioMu.Unlock()
				m.publishEvent(msg)
				if waitOrStop(rt.stopCh, backoff) {
					return
				}
				continue
			}
			for k, v := range setup {
				rt.state.StepResults[k] = v
			}
			rt.ioMu.Unlock()
			setupDone = true
		}

		if strings.ToLower(rt.template.ProtocolType) == "mqtt" {
			if waitOrStop(rt.stopCh, maxDuration(time.Duration(rt.device.PollInterval*float64(time.Second)), time.Second)) {
				return
			}
			continue
		}

		templateMap := store.JSONMapToMap(rt.template.Template)
		if !templateHasPollSteps(templateMap) {
			rt.ioMu.Lock()
			category := normalizeCategory(rt.device.DeviceCategory)
			eventType := eventTypeForCategory(category)
			if rt.state.Status != "online" {
				rt.state.MarkOnline(rt.state.Weight, normalizeUnit(rt.state.Unit), eventType, rt.state.Payload)
				m.publishEvent(rt.state.ToMessage())
			}
			rt.ioMu.Unlock()
			if waitOrStop(rt.stopCh, maxDuration(time.Duration(rt.device.PollInterval*float64(time.Second)), 200*time.Millisecond)) {
				return
			}
			continue
		}

		stage = "poll"
		rt.ioMu.Lock()
		steps, err := m.executor.RunPollSteps(context.Background(), templateMap, rt.driver, store.JSONMapToMap(rt.device.TemplateVariables), rt.state.StepResults)
		if err == nil {
			rt.state.StepResults = steps
			contextMap := map[string]any{"steps": steps}
			for k, v := range store.JSONMapToMap(rt.device.TemplateVariables) {
				contextMap[k] = v
			}
			output := m.executor.RenderOutput(templateMap, contextMap)
			weight := toFloatPtr(output["weight"])
			unit := normalizeUnit(fmt.Sprintf("%v", mgrValueOr(output["unit"], "kg")))
			category := normalizeCategory(rt.device.DeviceCategory)
			eventType := eventTypeForCategory(category)
			payload := buildRuntimePayload(output)
			shouldPublish := true
			if category == "scanner" {
				shouldPublish = scannerShouldPublish(rt, payload)
			}
			rt.state.MarkOnline(weight, unit, eventType, payload)
			if shouldPublish {
				m.publishEvent(rt.state.ToMessage())
			}
		}
		rt.ioMu.Unlock()
		if err != nil {
			m.metrics.IncPollError()
			m.metrics.RecordPollLatency(time.Since(cycleStart))
			rt.ioMu.Lock()
			rt.state.MarkError(formatException(stage, rt, err))
			msg := rt.state.ToMessage()
			rt.ioMu.Unlock()
			m.publishEvent(msg)
			if waitOrStop(rt.stopCh, backoff) {
				return
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}

		m.metrics.IncPollCycle()
		m.metrics.RecordPollLatency(time.Since(cycleStart))
		interval := maxDuration(time.Duration(rt.device.PollInterval*float64(time.Second)), 20*time.Millisecond)
		elapsed := time.Since(cycleStart)
		sleepFor := interval - elapsed
		if sleepFor < 5*time.Millisecond {
			sleepFor = 5 * time.Millisecond
		}
		if waitOrStop(rt.stopCh, sleepFor) {
			return
		}
	}
}

func (m *DeviceManager) handleMqttMessage(rt *deviceRuntime, topic string, payload []byte) {
	started := time.Now()
	rt.ioMu.Lock()
	defer rt.ioMu.Unlock()
	steps, output, err := m.executor.RunMessageHandler(context.Background(), store.JSONMapToMap(rt.template.Template), rt.driver, payload, store.JSONMapToMap(rt.device.TemplateVariables), rt.state.StepResults)
	if err != nil {
		m.metrics.IncPollError()
		rt.state.MarkError(formatException("mqtt_message:"+topic, rt, err))
		m.publishEvent(rt.state.ToMessage())
		return
	}
	m.metrics.IncPollCycle()
	m.metrics.IncMqttMessage()
	rt.state.StepResults = steps
	weight := toFloatPtr(output["weight"])
	unit := normalizeUnit(fmt.Sprintf("%v", mgrValueOr(output["unit"], "kg")))
	category := normalizeCategory(rt.device.DeviceCategory)
	eventType := eventTypeForCategory(category)
	payloadMap := buildRuntimePayload(output)
	rt.state.MarkOnline(weight, unit, eventType, payloadMap)
	m.publishEvent(rt.state.ToMessage())
	m.metrics.RecordPollLatency(time.Since(started))
}

func offlineRuntimeMessage() map[string]any {
	return map[string]any{
		"type":            "weight_update",
		"device_category": "weight",
		"status":          "offline",
		"weight":          nil,
		"unit":            "kg",
		"payload":         map[string]any{},
		"timestamp":       nil,
		"error":           nil,
	}
}

func normalizeCategory(v string) string {
	s := strings.ToLower(strings.TrimSpace(v))
	switch s {
	case "printer_tsc", "scanner", "serial_board", "weight":
		return s
	default:
		return "weight"
	}
}

func normalizeUnit(v string) string {
	s := strings.TrimSpace(v)
	if s == "" {
		return "kg"
	}
	return s
}

func eventTypeForCategory(category string) string {
	switch category {
	case "printer_tsc":
		return "print_event"
	case "scanner":
		return "scan_event"
	case "serial_board":
		return "board_event"
	default:
		return "weight_update"
	}
}

func buildRuntimePayload(output map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range output {
		if k == "weight" || k == "unit" {
			continue
		}
		out[k] = v
	}
	return out
}

func scannerShouldPublish(rt *deviceRuntime, payload map[string]any) bool {
	rt.dedupeMu.Lock()
	defer rt.dedupeMu.Unlock()
	barcode := strings.TrimSpace(fmt.Sprintf("%v", payload["barcode"]))
	if barcode == "" {
		payload["deduped"] = false
		return true
	}
	windowMS := scannerDedupeWindow(rt)
	now := time.Now()
	if barcode == rt.lastScanCode && !rt.lastScanAt.IsZero() && now.Sub(rt.lastScanAt) < time.Duration(windowMS)*time.Millisecond {
		payload["deduped"] = true
		payload["dedupe_window_ms"] = windowMS
		return false
	}
	rt.lastScanCode = barcode
	rt.lastScanAt = now
	payload["deduped"] = false
	payload["dedupe_window_ms"] = windowMS
	return true
}

func scannerDedupeWindow(rt *deviceRuntime) int {
	params := store.JSONMapToMap(rt.device.ConnectionParams)
	vars := store.JSONMapToMap(rt.device.TemplateVariables)
	v := toInt(mgrValueOr(params["dedupe_window_ms"], vars["dedupe_window_ms"]), 500)
	if v < 300 {
		return 300
	}
	if v > 800 {
		return 800
	}
	return v
}

func templateHasPollSteps(template map[string]any) bool {
	steps, _ := template["steps"].([]any)
	for _, raw := range steps {
		step, _ := raw.(map[string]any)
		if step == nil {
			continue
		}
		if fmt.Sprintf("%v", mgrValueOr(step["trigger"], "poll")) == "poll" {
			return true
		}
	}
	return false
}

func formatError(stage string, rt *deviceRuntime, errType, errText string) string {
	return fmt.Sprintf("stage=%s; protocol=%s; endpoint=%s; error_type=%s; error=%s", stage, strings.ToLower(rt.template.ProtocolType), runtimeEndpoint(rt), errType, errText)
}

func formatException(stage string, rt *deviceRuntime, err error) string {
	return fmt.Sprintf("stage=%s; protocol=%s; endpoint=%s; error_type=%T; error=%v", stage, strings.ToLower(rt.template.ProtocolType), runtimeEndpoint(rt), err, err)
}

func runtimeEndpoint(rt *deviceRuntime) string {
	params := store.JSONMapToMap(rt.device.ConnectionParams)
	host := strings.TrimSpace(fmt.Sprintf("%v", mgrValueOr(params["host"], "")))
	port := strings.TrimSpace(fmt.Sprintf("%v", mgrValueOr(params["port"], "")))
	if host != "" && port != "" {
		return host + ":" + port
	}
	if host != "" {
		return host
	}
	if port != "" {
		return port
	}
	return "-"
}

func toFloatPtr(v any) *float64 {
	if v == nil {
		return nil
	}
	f := toFloat(v)
	return &f
}

func toFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case string:
		var out float64
		_, _ = fmt.Sscanf(strings.TrimSpace(t), "%f", &out)
		return out
	default:
		return 0
	}
}

func toInt(v any, fallback int) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		var out int
		if _, err := fmt.Sscanf(strings.TrimSpace(t), "%d", &out); err == nil {
			return out
		}
	}
	return fallback
}

func mgrValueOr(v, fallback any) any {
	if v == nil {
		return fallback
	}
	return v
}

func waitOrStop(stopCh <-chan struct{}, d time.Duration) bool {
	if d <= 0 {
		select {
		case <-stopCh:
			return true
		default:
			return false
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-stopCh:
		return true
	case <-timer.C:
		return false
	}
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
