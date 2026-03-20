package service

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type RuntimeMetrics struct {
	pollCycles      atomic.Uint64
	pollErrors      atomic.Uint64
	mqttMessages    atomic.Uint64
	reconnects      atomic.Uint64
	runtimeRestarts atomic.Uint64
	publishedEvents atomic.Uint64

	latMu       sync.RWMutex
	latencyRing []time.Duration
	latIndex    int
	latFilled   bool
}

func NewRuntimeMetrics(samples int) *RuntimeMetrics {
	if samples < 256 {
		samples = 256
	}
	return &RuntimeMetrics{
		latencyRing: make([]time.Duration, samples),
	}
}

func (m *RuntimeMetrics) IncPollCycle()      { m.pollCycles.Add(1) }
func (m *RuntimeMetrics) IncPollError()      { m.pollErrors.Add(1) }
func (m *RuntimeMetrics) IncReconnect()      { m.reconnects.Add(1) }
func (m *RuntimeMetrics) IncRuntimeRestart() { m.runtimeRestarts.Add(1) }
func (m *RuntimeMetrics) IncMqttMessage()    { m.mqttMessages.Add(1) }
func (m *RuntimeMetrics) IncPublishedEvent() { m.publishedEvents.Add(1) }

func (m *RuntimeMetrics) RecordPollLatency(d time.Duration) {
	if d < 0 {
		d = 0
	}
	m.latMu.Lock()
	m.latencyRing[m.latIndex] = d
	m.latIndex++
	if m.latIndex >= len(m.latencyRing) {
		m.latIndex = 0
		m.latFilled = true
	}
	m.latMu.Unlock()
}

func (m *RuntimeMetrics) Snapshot(busStats map[string]any) map[string]any {
	values := m.latencySnapshot()
	p50 := percentile(values, 50)
	p95 := percentile(values, 95)
	p99 := percentile(values, 99)
	return map[string]any{
		"poll_cycles":       m.pollCycles.Load(),
		"poll_errors":       m.pollErrors.Load(),
		"mqtt_messages":     m.mqttMessages.Load(),
		"reconnects":        m.reconnects.Load(),
		"runtime_restarts":  m.runtimeRestarts.Load(),
		"published_events":  m.publishedEvents.Load(),
		"latency_samples":   len(values),
		"latency_p50_ms":    durationToMS(p50),
		"latency_p95_ms":    durationToMS(p95),
		"latency_p99_ms":    durationToMS(p99),
		"event_bus_stats":   busStats,
		"slo_p95_target_ms": 20.0,
	}
}

func (m *RuntimeMetrics) latencySnapshot() []time.Duration {
	m.latMu.RLock()
	defer m.latMu.RUnlock()
	size := m.latIndex
	if m.latFilled {
		size = len(m.latencyRing)
	}
	out := make([]time.Duration, 0, size)
	for i := 0; i < size; i++ {
		out = append(out, m.latencyRing[i])
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func percentile(values []time.Duration, p int) time.Duration {
	if len(values) == 0 {
		return 0
	}
	if p <= 0 {
		return values[0]
	}
	if p >= 100 {
		return values[len(values)-1]
	}
	idx := int(float64(len(values)-1) * (float64(p) / 100.0))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(values) {
		idx = len(values) - 1
	}
	return values[idx]
}

func durationToMS(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}
