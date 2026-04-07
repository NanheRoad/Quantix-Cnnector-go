package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quantix-connector-go/internal/config"
	"quantix-connector-go/internal/service"
	"quantix-connector-go/internal/store"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func TestProtectedAPIKey(t *testing.T) {
	ts, _, cfg := newTestServer(t)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/devices", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	req2, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/devices", nil)
	req2.Header.Set("X-API-Key", cfg.APIKey)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("request with key failed: %v", err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	_ = resp2.Body.Close()
}

func TestDeviceByCodeAndDisabledExecute(t *testing.T) {
	ts, _, cfg := newTestServer(t)

	protocols := []map[string]any{}
	getJSON(t, ts.URL+"/api/protocols", cfg.APIKey, &protocols)
	if len(protocols) == 0 {
		t.Fatalf("expected seeded protocols")
	}
	protocolID, ok := protocols[0]["id"].(float64)
	if !ok {
		t.Fatalf("unexpected protocol id type: %T", protocols[0]["id"])
	}

	create := map[string]any{
		"device_code":          "WGT-001",
		"device_category":      "weight",
		"name":                 "Scale-A",
		"protocol_template_id": int(protocolID),
		"connection_params":    map[string]any{"host": "127.0.0.1", "port": 1502},
		"template_variables":   map[string]any{"slave_id": 1, "address": 0},
		"poll_interval":        0.05,
		"enabled":              false,
	}
	status, body := postJSON(t, ts.URL+"/api/devices", cfg.APIKey, create)
	if status != http.StatusOK {
		t.Fatalf("create device expected 200, got %d, body=%s", status, string(body))
	}

	item := map[string]any{}
	getJSON(t, ts.URL+"/api/devices/by-code/WGT-001", cfg.APIKey, &item)
	if got := strings.TrimSpace(toString(item["device_code"])); got != "WGT-001" {
		t.Fatalf("unexpected device_code: %v", item["device_code"])
	}

	status, body = postJSON(t, ts.URL+"/api/devices/by-code/WGT-001/disable", cfg.APIKey, map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("disable expected 200, got %d, body=%s", status, string(body))
	}

	execPayload := map[string]any{"step_id": "tare", "params": map[string]any{}}
	status, body = postJSON(t, ts.URL+"/api/devices/by-code/WGT-001/execute", cfg.APIKey, execPayload)
	if status != http.StatusBadRequest {
		t.Fatalf("execute disabled expected 400, got %d, body=%s", status, string(body))
	}
}

func TestHealthContainsIndustrialMetrics(t *testing.T) {
	ts, _, _ := newTestServer(t)

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if _, ok := body["status"]; !ok {
		t.Fatalf("missing status in health")
	}
	metrics, ok := body["metrics"].(map[string]any)
	if !ok {
		t.Fatalf("missing metrics object")
	}
	if _, ok := metrics["latency_p95_ms"]; !ok {
		t.Fatalf("missing latency_p95_ms")
	}
}

func TestWebSocketUnauthorizedCloseCode(t *testing.T) {
	ts, _, _ := newTestServer(t)

	u := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws?api_key=wrong-key"
	conn, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("ws dial failed: %v", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Fatalf("expected close error")
	}
	closeErr, ok := err.(*websocket.CloseError)
	if !ok {
		t.Fatalf("expected CloseError, got %T (%v)", err, err)
	}
	if closeErr.Code != 4401 {
		t.Fatalf("expected close code 4401, got %d", closeErr.Code)
	}
}

func TestStepTestWriteGateBlocksSerialSend(t *testing.T) {
	ts, _, cfg := newTestServer(t)

	protoPayload := map[string]any{
		"name":          "write-gate-serial",
		"description":   "write gate check",
		"protocol_type": "serial",
		"template": map[string]any{
			"steps": []any{
				map[string]any{
					"id":      "danger",
					"trigger": "poll",
					"action":  "serial.send",
					"params":  map[string]any{"data": "PING"},
				},
			},
			"output": map[string]any{},
		},
		"is_system": false,
	}
	status, body := postJSON(t, ts.URL+"/api/protocols", cfg.APIKey, protoPayload)
	if status != http.StatusOK {
		t.Fatalf("create protocol expected 200, got %d, body=%s", status, string(body))
	}
	created := map[string]any{}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("decode create protocol failed: %v", err)
	}
	pid := int(created["id"].(float64))

	stepReq := map[string]any{
		"connection_params":  map[string]any{"port": "COM1", "baudrate": 9600},
		"template_variables": map[string]any{},
		"step_id":            "danger",
		"step_context":       "poll",
		"allow_write":        false,
		"previous_steps":     map[string]any{},
	}
	status, body = postJSON(t, fmt.Sprintf("%s/api/protocols/%d/test-step", ts.URL, pid), cfg.APIKey, stepReq)
	if status != http.StatusOK {
		t.Fatalf("test-step expected 200, got %d, body=%s", status, string(body))
	}
	result := map[string]any{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode test-step failed: %v", err)
	}
	if ok, _ := result["ok"].(bool); ok {
		t.Fatalf("expected write gate to block serial.send, body=%s", string(body))
	}
	if strings.TrimSpace(toString(result["safety_warning"])) == "" {
		t.Fatalf("expected safety_warning in body=%s", string(body))
	}
}

func TestServeIndexNotDependentOnCWD(t *testing.T) {
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	defer func() { _ = os.Chdir(orig) }()

	ts, _, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(strings.ToLower(string(body)), "<!doctype html>") {
		t.Fatalf("unexpected index body")
	}
}

func newTestServer(t *testing.T) (*httptest.Server, *service.DeviceManager, config.Settings) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := config.Settings{
		DBType:                "sqlite",
		DBName:                filepath.Join(t.TempDir(), "quantix-test.db"),
		APIKey:                "test-key",
		BackendHost:           "127.0.0.1",
		BackendPort:           0,
		SimulateOnConnectFail: true,
	}
	db, err := store.OpenDB(cfg)
	if err != nil {
		t.Fatalf("open db failed: %v", err)
	}
	manager := service.NewDeviceManager(db, cfg)
	serialDebug := service.NewSerialDebugService()
	printAgent := service.NewPrintAgentService(cfg.PrintAgent)
	srv := NewServer(cfg, db, manager, serialDebug, printAgent)
	ts := httptest.NewServer(srv.Router())

	t.Cleanup(func() {
		ts.Close()
		_ = manager.Shutdown(context.Background())
		sqlDB, e := db.DB()
		if e == nil {
			_ = sqlDB.Close()
		}
	})
	return ts, manager, cfg
}

func getJSON(t *testing.T, url, apiKey string, out any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("X-API-Key", apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s expected 200, got %d", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode %s failed: %v", url, err)
	}
}

func postJSON(t *testing.T, url, apiKey string, payload any) (int, []byte) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	body := []byte{}
	body, _ = io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}
