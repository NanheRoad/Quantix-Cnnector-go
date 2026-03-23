package driver

import (
	"context"
	"fmt"
	"sync"
	"time"

	"quantix-connector-go/internal/config"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type MqttDriver struct {
	cfg       config.Settings
	params    map[string]any
	client    mqtt.Client
	connected bool
	lastErr   string
	handler   MessageHandler
	mu        sync.RWMutex
}

func NewMqttDriver(params map[string]any, cfg config.Settings) *MqttDriver {
	return &MqttDriver{params: params, cfg: cfg}
}

func (d *MqttDriver) RegisterMessageHandler(handler MessageHandler) {
	d.mu.Lock()
	d.handler = handler
	d.mu.Unlock()
}

func (d *MqttDriver) Connect(ctx context.Context) (bool, error) {
	d.mu.Lock()
	d.lastErr = ""
	oldClient := d.client
	d.client = nil
	d.connected = false
	host := asString(d.params, "host", "127.0.0.1")
	port := asInt(d.params, "port", 1883)
	username := asString(d.params, "username", "")
	password := asString(d.params, "password", "")
	d.mu.Unlock()
	if oldClient != nil && oldClient.IsConnectionOpen() {
		oldClient.Disconnect(100)
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", host, port))
	opts.SetClientID(fmt.Sprintf("quantix-%d", time.Now().UnixNano()))
	opts.SetAutoReconnect(false)
	if username != "" {
		opts.SetUsername(username)
		opts.SetPassword(password)
	}
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		d.mu.Lock()
		d.connected = true
		d.lastErr = ""
		d.mu.Unlock()
	})
	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		d.mu.Lock()
		d.connected = false
		if err != nil {
			d.lastErr = err.Error()
		}
		d.mu.Unlock()
	})

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if ok := token.WaitTimeout(2 * time.Second); !ok || token.Error() != nil {
		err := token.Error()
		if err == nil {
			err = fmt.Errorf("mqtt connect timeout")
		}
		d.mu.Lock()
		d.lastErr = fmt.Sprintf("mqtt connect failed: %v", err)
		d.connected = false
		d.mu.Unlock()
		if d.cfg.SimulateOnConnectFail {
			d.mu.Lock()
			d.connected = true
			d.mu.Unlock()
			return true, nil
		}
		return false, nil
	}

	d.mu.Lock()
	d.client = client
	d.connected = true
	d.lastErr = ""
	d.mu.Unlock()
	_ = ctx
	return true, nil
}

func (d *MqttDriver) Disconnect(ctx context.Context) error {
	d.mu.Lock()
	client := d.client
	d.client = nil
	d.connected = false
	d.mu.Unlock()

	if client != nil && client.IsConnectionOpen() {
		client.Disconnect(100)
	}
	_ = ctx
	return nil
}

func (d *MqttDriver) IsConnected() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.connected
}

func (d *MqttDriver) LastError() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.lastErr
}

func (d *MqttDriver) ExecuteAction(ctx context.Context, action string, params map[string]any) (any, error) {
	d.mu.RLock()
	client := d.client
	d.mu.RUnlock()

	switch action {
	case "mqtt.subscribe":
		topic := asString(params, "topic", "")
		qos := byte(asInt(params, "qos", 0))
		if client != nil {
			token := client.Subscribe(topic, qos, func(client mqtt.Client, msg mqtt.Message) {
				handler := d.getHandler()
				if handler != nil {
					handler(msg.Topic(), append([]byte{}, msg.Payload()...))
				}
			})
			_ = token.WaitTimeout(1 * time.Second)
			if err := token.Error(); err != nil {
				return nil, err
			}
		} else if !d.cfg.SimulateOnConnectFail {
			return nil, fmt.Errorf(d.lastErrOr("mqtt client is not connected"))
		}
		return map[string]any{"topic": topic, "qos": int(qos)}, nil
	case "mqtt.publish":
		topic := asString(params, "topic", "")
		payload := asString(params, "payload", "")
		qos := byte(asInt(params, "qos", 0))
		if client != nil {
			token := client.Publish(topic, qos, false, payload)
			_ = token.WaitTimeout(1 * time.Second)
			if err := token.Error(); err != nil {
				return nil, err
			}
		} else if !d.cfg.SimulateOnConnectFail {
			return nil, fmt.Errorf(d.lastErrOr("mqtt client is not connected"))
		}
		return map[string]any{"topic": topic, "published": true}, nil
	case "mqtt.on_message":
		_ = ctx
		return map[string]any{"ok": true}, nil
	default:
		return nil, fmt.Errorf("unsupported action for MqttDriver: %s", action)
	}
}

func (d *MqttDriver) getHandler() MessageHandler {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.handler
}

func (d *MqttDriver) lastErrOr(fallback string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.lastErr != "" {
		return d.lastErr
	}
	return fallback
}
