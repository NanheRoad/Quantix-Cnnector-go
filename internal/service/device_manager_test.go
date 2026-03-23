package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"quantix-connector-go/internal/config"
	"quantix-connector-go/internal/driver"
	"quantix-connector-go/internal/store"
)

type fakeRuntimeDriver struct {
	mu             sync.Mutex
	connected      bool
	subscribeCalls int
}

func (d *fakeRuntimeDriver) Connect(ctx context.Context) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.connected = true
	return true, nil
}

func (d *fakeRuntimeDriver) Disconnect(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.connected = false
	return nil
}

func (d *fakeRuntimeDriver) IsConnected() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.connected
}

func (d *fakeRuntimeDriver) ExecuteAction(ctx context.Context, action string, params map[string]any) (any, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch action {
	case "mqtt.subscribe":
		d.subscribeCalls++
		return map[string]any{"ok": true}, nil
	default:
		return nil, errors.New("unexpected action")
	}
}

func (d *fakeRuntimeDriver) RegisterMessageHandler(handler driver.MessageHandler) {}

func (d *fakeRuntimeDriver) LastError() string { return "" }

func (d *fakeRuntimeDriver) forceDisconnect() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.connected = false
}

func (d *fakeRuntimeDriver) subscribeCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.subscribeCalls
}

func TestStopDeviceTimeoutRetainsRuntimeForRetry(t *testing.T) {
	manager := NewDeviceManager(nil, config.Settings{})
	runCtx, cancel := context.WithCancel(context.Background())
	rt := &deviceRuntime{
		device: store.Device{ID: 1},
		driver: &fakeRuntimeDriver{},
		state:  NewRuntimeState(1, "dev", "dev-1", "weight"),
		runCtx: runCtx,
		cancel: cancel,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
	manager.runtimes[1] = rt

	err := manager.stopDeviceLocked(context.Background(), 1)
	if err == nil {
		t.Fatal("expected stop timeout error")
	}
	if got := manager.GetRuntime(1); got != nil {
		t.Fatal("expected stopping runtime to be hidden from callers")
	}
	manager.mu.Lock()
	kept := manager.runtimes[1]
	manager.mu.Unlock()
	if kept != rt {
		t.Fatal("expected timed out runtime to remain tracked")
	}
}

func TestRunRuntimeLoopRerunsSetupAfterReconnect(t *testing.T) {
	manager := NewDeviceManager(nil, config.Settings{})
	drv := &fakeRuntimeDriver{}
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt := &deviceRuntime{
		device: store.Device{
			ID:           1,
			Name:         "mqtt-dev",
			DeviceCode:   "mqtt-dev",
			PollInterval: 0.05,
		},
		template: store.ProtocolTemplate{
			ProtocolType: "mqtt",
			Template: store.ToJSONMap(map[string]any{
				"setup_steps": []any{
					map[string]any{
						"id":     "sub",
						"action": "mqtt.subscribe",
						"params": map[string]any{"topic": "factory/topic"},
					},
				},
			}),
		},
		driver: drv,
		state:  NewRuntimeState(1, "mqtt-dev", "mqtt-dev", "weight"),
		runCtx: runCtx,
		cancel: cancel,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		manager.runRuntimeLoop(rt)
	}()
	defer func() {
		close(rt.stopCh)
		<-done
	}()

	waitForCondition(t, time.Second, func() bool { return drv.subscribeCount() >= 1 })
	drv.forceDisconnect()
	waitForCondition(t, 2*time.Second, func() bool { return drv.subscribeCount() >= 2 })
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}
