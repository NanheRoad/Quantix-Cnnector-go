package driver

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"quantix-connector-go/internal/config"

	"go.bug.st/serial"
)

type SerialDriver struct {
	cfg       config.Settings
	params    map[string]any
	port      serial.Port
	connected bool
	lastErr   string
	mu        sync.Mutex
}

func NewSerialDriver(params map[string]any, cfg config.Settings) *SerialDriver {
	return &SerialDriver{params: params, cfg: cfg}
}

func (d *SerialDriver) Connect(ctx context.Context) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastErr = ""
	portName := asString(d.params, "port", "")
	if portName == "" {
		d.lastErr = "serial port is required"
		if d.cfg.SimulateOnConnectFail {
			d.connected = true
			return true, nil
		}
		d.connected = false
		return false, nil
	}
	mode := &serial.Mode{
		BaudRate: asInt(d.params, "baudrate", 9600),
		DataBits: asInt(d.params, "bytesize", 8),
		Parity:   parseParity(asString(d.params, "parity", "N")),
		StopBits: parseStopBits(asInt(d.params, "stopbits", 1)),
	}
	p, err := serial.Open(portName, mode)
	if err != nil {
		d.lastErr = fmt.Sprintf("serial open failed: %v", err)
		if d.cfg.SimulateOnConnectFail {
			d.connected = true
			return true, nil
		}
		d.connected = false
		return false, nil
	}
	_ = p.SetReadTimeout(time.Duration(asFloat(d.params, "timeout", 1.0)*1000) * time.Millisecond)
	d.port = p
	d.connected = true
	_ = ctx
	return true, nil
}

func (d *SerialDriver) Disconnect(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.port != nil {
		_ = d.port.Close()
	}
	d.port = nil
	d.connected = false
	_ = ctx
	return nil
}

func (d *SerialDriver) IsConnected() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.connected
}

func (d *SerialDriver) LastError() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lastErr
}

func (d *SerialDriver) RegisterMessageHandler(handler MessageHandler) {
	_ = handler
}

func (d *SerialDriver) ExecuteAction(ctx context.Context, action string, params map[string]any) (any, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch action {
	case "serial.send":
		return d.send(params)
	case "serial.receive":
		return d.receive(ctx, params)
	default:
		return nil, fmt.Errorf("unsupported action for SerialDriver: %s", action)
	}
}

func (d *SerialDriver) send(params map[string]any) (any, error) {
	data, err := toBytes(params["data"], asString(params, "encoding", "ascii"))
	if err != nil {
		return nil, err
	}
	if d.port == nil {
		return nil, fmt.Errorf("serial send failed: serial port is not connected")
	}
	n, err := d.port.Write(data)
	if err != nil {
		d.connected = false
		d.lastErr = fmt.Sprintf("serial send failed: %v", err)
		return nil, err
	}
	return map[string]any{"bytes_sent": n}, nil
}

func (d *SerialDriver) receive(ctx context.Context, params map[string]any) (any, error) {
	if d.port == nil {
		return nil, fmt.Errorf("serial receive failed: serial port is not connected")
	}
	size := asInt(params, "size", asInt(params, "max_bytes", 1024))
	timeoutMs := asInt(params, "timeout", 1000)
	delimiter := []byte(decodeEscapes(asString(params, "delimiter", "")))
	buf := bytes.Buffer{}
	tmp := make([]byte, 256)
	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	for buf.Len() < size {
		if dl, ok := ctx.Deadline(); ok {
			deadline = dl
		}
		if time.Now().After(deadline) {
			break
		}
		n, err := d.port.Read(tmp)
		if err != nil {
			d.connected = false
			d.lastErr = fmt.Sprintf("serial receive failed: %v", err)
			return nil, err
		}
		if n > 0 {
			_, _ = buf.Write(tmp[:n])
			if len(delimiter) > 0 && bytes.Contains(buf.Bytes(), delimiter) {
				break
			}
			if len(delimiter) == 0 && buf.Len() >= size {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	payload := buf.Bytes()
	if len(delimiter) > 0 {
		if idx := bytes.Index(payload, delimiter); idx >= 0 {
			payload = payload[:idx+len(delimiter)]
		}
	}
	return map[string]any{"payload": append([]byte{}, payload...)}, nil
}

func parseParity(raw string) serial.Parity {
	switch raw {
	case "E":
		return serial.EvenParity
	case "O":
		return serial.OddParity
	default:
		return serial.NoParity
	}
}

func parseStopBits(raw int) serial.StopBits {
	switch raw {
	case 2:
		return serial.TwoStopBits
	default:
		return serial.OneStopBit
	}
}
