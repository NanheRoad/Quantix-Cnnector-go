package service

import (
	"context"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

type SerialDebugService struct {
	mu       sync.Mutex
	port     serial.Port
	settings map[string]any
	lastErr  string
	logs     []map[string]any
	logSeq   int
}

func NewSerialDebugService() *SerialDebugService {
	return &SerialDebugService{
		settings: map[string]any{},
		logs:     []map[string]any{},
	}
}

func (s *SerialDebugService) ListPorts(ctx context.Context) []map[string]any {
	_ = ctx
	out := []map[string]any{}
	ports, err := enumerator.GetDetailedPortsList()
	if err == nil && len(ports) > 0 {
		for _, p := range ports {
			out = append(out, map[string]any{
				"device":        p.Name,
				"name":          p.Name,
				"description":   p.Product,
				"hwid":          p.IsUSB,
				"manufacturer":  "",
				"serial_number": p.SerialNumber,
			})
		}
		sort.Slice(out, func(i, j int) bool {
			return fmt.Sprintf("%v", out[i]["device"]) < fmt.Sprintf("%v", out[j]["device"])
		})
		return out
	}

	fallback, err := serial.GetPortsList()
	if err != nil {
		return out
	}
	for _, name := range fallback {
		out = append(out, map[string]any{
			"device":        name,
			"name":          name,
			"description":   "",
			"hwid":          "",
			"manufacturer":  "",
			"serial_number": "",
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return fmt.Sprintf("%v", out[i]["device"]) < fmt.Sprintf("%v", out[j]["device"])
	})
	return out
}

func (s *SerialDebugService) Open(ctx context.Context, params map[string]any) (map[string]any, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.port != nil {
		_ = s.port.Close()
		s.port = nil
	}
	portName := strings.TrimSpace(fmt.Sprintf("%v", params["port"]))
	if portName == "" {
		return nil, fmt.Errorf("port is required")
	}
	mode := &serial.Mode{
		BaudRate: int(getFloat(params["baudrate"], 9600)),
		DataBits: int(getFloat(params["bytesize"], 8)),
		Parity:   parseDbgParity(fmt.Sprintf("%v", dbgValueOr(params["parity"], "N"))),
		StopBits: parseDbgStopBits(getFloat(params["stopbits"], 1)),
	}
	p, err := serial.Open(portName, mode)
	if err != nil {
		s.lastErr = err.Error()
		s.appendLog("ERR", []byte{}, "Open failed: "+err.Error())
		return nil, fmt.Errorf("open failed: %w", err)
	}
	_ = p.SetReadTimeout(time.Duration(getFloat(params["timeout_ms"], 300)) * time.Millisecond)
	s.port = p
	s.settings = map[string]any{
		"port":       portName,
		"baudrate":   mode.BaudRate,
		"bytesize":   mode.DataBits,
		"parity":     fmt.Sprintf("%v", dbgValueOr(params["parity"], "N")),
		"stopbits":   getFloat(params["stopbits"], 1),
		"timeout_ms": getFloat(params["timeout_ms"], 300),
	}
	s.lastErr = ""
	s.appendLog("SYS", []byte{}, fmt.Sprintf("Connected: %s %d/%d/%v/%v", portName, mode.BaudRate, mode.DataBits, s.settings["parity"], s.settings["stopbits"]))
	return s.statusUnlocked(), nil
}

func (s *SerialDebugService) Close(ctx context.Context) map[string]any {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.port != nil {
		_ = s.port.Close()
		s.port = nil
	}
	s.appendLog("SYS", []byte{}, "Disconnected")
	return s.statusUnlocked()
}

func (s *SerialDebugService) Status(ctx context.Context) map[string]any {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusUnlocked()
}

func (s *SerialDebugService) Send(ctx context.Context, data, dataFormat, encoding, lineEnding string) (map[string]any, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.port == nil {
		return nil, fmt.Errorf("serial debugger is not connected")
	}
	payload, err := buildPayload(data, dataFormat, lineEnding)
	if err != nil {
		return nil, err
	}
	n, err := s.port.Write(payload)
	if err != nil {
		s.lastErr = err.Error()
		return nil, err
	}
	text := string(payload)
	if strings.ToLower(dataFormat) == "hex" {
		text = strings.ToUpper(hex.EncodeToString(payload))
	}
	s.appendLog("TX", payload, text)
	return map[string]any{
		"ok":          true,
		"bytes_sent":  n,
		"payload_hex": fmt.Sprintf("% x", payload),
		"timestamp":   time.Now().UTC().Format(time.RFC3339Nano),
		"encoding":    encoding,
	}, nil
}

func (s *SerialDebugService) Read(ctx context.Context, maxBytes, timeoutMs int, encoding string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.port == nil {
		return nil, fmt.Errorf("serial debugger is not connected")
	}
	if maxBytes <= 0 {
		maxBytes = 1
	}
	if timeoutMs < 0 {
		timeoutMs = 0
	}
	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	buf := make([]byte, 0, maxBytes)
	tmp := make([]byte, 512)
	for len(buf) < maxBytes {
		select {
		case <-ctx.Done():
			return map[string]any{
				"ok":           true,
				"bytes_read":   len(buf),
				"payload_text": string(buf),
				"payload_hex":  fmt.Sprintf("% x", buf),
				"timestamp":    time.Now().UTC().Format(time.RFC3339Nano),
				"encoding":     encoding,
			}, nil
		default:
		}
		if time.Now().After(deadline) {
			break
		}
		n, err := s.port.Read(tmp)
		if err != nil {
			s.lastErr = err.Error()
			return nil, fmt.Errorf("serial read failed: %w", err)
		}
		if n > 0 {
			remain := maxBytes - len(buf)
			if n > remain {
				n = remain
			}
			buf = append(buf, tmp[:n]...)
			if len(buf) >= maxBytes {
				break
			}
			continue
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(buf) > 0 {
		s.appendLog("RX", buf, string(buf))
	}
	return map[string]any{
		"ok":           true,
		"bytes_read":   len(buf),
		"payload_text": string(buf),
		"payload_hex":  fmt.Sprintf("% x", buf),
		"timestamp":    time.Now().UTC().Format(time.RFC3339Nano),
		"encoding":     encoding,
	}, nil
}

func (s *SerialDebugService) PullLogs(ctx context.Context, lastSeq, limit int) map[string]any {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 1
	}
	if limit > 500 {
		limit = 500
	}
	entries := []map[string]any{}
	for _, item := range s.logs {
		seq := int(getFloat(item["seq"], 0))
		if seq > lastSeq {
			entries = append(entries, item)
		}
	}
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	return map[string]any{
		"ok":       true,
		"entries":  entries,
		"next_seq": s.logSeq,
	}
}

func (s *SerialDebugService) statusUnlocked() map[string]any {
	return map[string]any{
		"ok":         true,
		"connected":  s.port != nil,
		"settings":   cloneMap(s.settings),
		"last_error": valueOrNilString(s.lastErr),
	}
}

func (s *SerialDebugService) appendLog(direction string, payload []byte, text string) {
	s.logSeq++
	entry := map[string]any{
		"seq":       s.logSeq,
		"direction": direction,
		"bytes":     len(payload),
		"text":      text,
		"hex":       fmt.Sprintf("% x", payload),
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}
	s.logs = append(s.logs, entry)
	if len(s.logs) > 1000 {
		s.logs = s.logs[len(s.logs)-1000:]
	}
}

func parseDbgParity(v string) serial.Parity {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "E":
		return serial.EvenParity
	case "O":
		return serial.OddParity
	default:
		return serial.NoParity
	}
}

func parseDbgStopBits(v float64) serial.StopBits {
	if v >= 2 {
		return serial.TwoStopBits
	}
	return serial.OneStopBit
}

func buildPayload(data, dataFormat, lineEnding string) ([]byte, error) {
	var raw []byte
	var err error
	if strings.ToLower(strings.TrimSpace(dataFormat)) == "hex" {
		clean := strings.ReplaceAll(strings.TrimSpace(data), " ", "")
		raw, err = hex.DecodeString(clean)
		if err != nil {
			return nil, fmt.Errorf("invalid hex payload: %w", err)
		}
	} else {
		raw = []byte(data)
	}
	switch strings.ToLower(strings.TrimSpace(lineEnding)) {
	case "cr":
		raw = append(raw, '\r')
	case "lf":
		raw = append(raw, '\n')
	case "crlf":
		raw = append(raw, '\r', '\n')
	}
	return raw, nil
}

func getFloat(v any, fallback float64) float64 {
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
		if _, err := fmt.Sscanf(strings.TrimSpace(t), "%f", &out); err == nil {
			return out
		}
	}
	return fallback
}

func dbgValueOr(v, fallback any) any {
	if v == nil {
		return fallback
	}
	return v
}

func valueOrNilString(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}
