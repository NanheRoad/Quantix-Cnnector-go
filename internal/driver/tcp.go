package driver

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"sync"
	"time"

	"quantix-connector-go/internal/config"
)

type TCPDriver struct {
	cfg        config.Settings
	params     map[string]any
	conn       net.Conn
	connected  bool
	lastErr    string
	mu         sync.Mutex
	msgHandler MessageHandler
}

func NewTCPDriver(params map[string]any, cfg config.Settings) *TCPDriver {
	return &TCPDriver{params: params, cfg: cfg}
}

func (d *TCPDriver) Connect(ctx context.Context) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastErr = ""
	host := asString(d.params, "host", "")
	port := asInt(d.params, "port", 0)
	if host == "" || port <= 0 {
		d.lastErr = "tcp requires host and port"
		if d.cfg.SimulateOnConnectFail {
			d.connected = true
			return true, nil
		}
		d.connected = false
		return false, nil
	}

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 2*time.Second)
	if err != nil {
		d.lastErr = fmt.Sprintf("tcp connect failed: %v", err)
		if d.cfg.SimulateOnConnectFail {
			d.connected = true
			return true, nil
		}
		d.connected = false
		return false, nil
	}
	d.conn = conn
	d.connected = true
	return true, nil
}

func (d *TCPDriver) Disconnect(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn != nil {
		_ = d.conn.Close()
	}
	d.conn = nil
	d.connected = false
	_ = ctx
	return nil
}

func (d *TCPDriver) IsConnected() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.connected
}

func (d *TCPDriver) LastError() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lastErr
}

func (d *TCPDriver) RegisterMessageHandler(handler MessageHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.msgHandler = handler
}

func (d *TCPDriver) ExecuteAction(ctx context.Context, action string, params map[string]any) (any, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch action {
	case "tcp.send":
		return d.send(ctx, params)
	case "tcp.receive":
		return d.receive(ctx, params)
	default:
		return nil, fmt.Errorf("unsupported action for TCPDriver: %s", action)
	}
}

func (d *TCPDriver) send(ctx context.Context, params map[string]any) (any, error) {
	data, err := toBytes(params["data"], asString(params, "encoding", "ascii"))
	if err != nil {
		return nil, err
	}
	waitAck := asBool(params, "wait_ack", false)
	ackRequired := asBool(params, "ack_required", false)
	ackTimeout := asInt(params, "ack_timeout", asInt(params, "timeout", 1000))
	ackSize := asInt(params, "ack_size", 128)
	ackPattern := asString(params, "ack_pattern", "")

	if d.conn == nil && !d.cfg.SimulateOnConnectFail {
		return nil, fmt.Errorf(d.lastErrOr("tcp writer is not connected"))
	}
	if d.conn != nil {
		if deadline, ok := ctx.Deadline(); ok {
			_ = d.conn.SetWriteDeadline(deadline)
		}
		_, err := d.conn.Write(data)
		if err != nil {
			d.connected = false
			d.lastErr = fmt.Sprintf("tcp send failed: %v", err)
			return nil, err
		}
	}

	ackOK := true
	ackBytes := []byte{}
	ackText := ""
	if waitAck {
		ackOK = false
		if d.conn == nil {
			if !d.cfg.SimulateOnConnectFail {
				return nil, fmt.Errorf(d.lastErrOr("tcp reader is not connected"))
			}
		} else {
			_ = d.conn.SetReadDeadline(time.Now().Add(time.Duration(ackTimeout) * time.Millisecond))
			buf := make([]byte, max(1, ackSize))
			n, readErr := d.conn.Read(buf)
			if readErr == nil && n > 0 {
				ackBytes = append([]byte{}, buf[:n]...)
				ackText = string(ackBytes)
				if ackPattern != "" {
					re, err := regexp.Compile(ackPattern)
					if err == nil {
						ackOK = re.MatchString(ackText)
					}
				} else {
					ackOK = true
				}
			}
		}
		if ackRequired && !ackOK {
			return nil, fmt.Errorf("tcp send ack timeout or pattern mismatch")
		}
	}

	return map[string]any{
		"bytes_sent":  len(data),
		"ack_ok":     ackOK,
		"ack_payload": fmt.Sprintf("% x", ackBytes),
		"ack_text":   ackText,
	}, nil
}

func (d *TCPDriver) receive(ctx context.Context, params map[string]any) (any, error) {
	if d.conn == nil {
		if d.cfg.SimulateOnConnectFail {
			return map[string]any{"payload": []byte("0.0")}, nil
		}
		return nil, fmt.Errorf(d.lastErrOr("tcp reader is not connected"))
	}
	size := asInt(params, "size", 64)
	timeoutMs := asInt(params, "timeout", 1000)
	if deadline, ok := ctx.Deadline(); ok {
		_ = d.conn.SetReadDeadline(deadline)
	} else {
		_ = d.conn.SetReadDeadline(time.Now().Add(time.Duration(timeoutMs) * time.Millisecond))
	}
	buf := make([]byte, max(1, size))
	n, err := d.conn.Read(buf)
	if err != nil {
		d.connected = false
		d.lastErr = fmt.Sprintf("tcp receive failed: %v", err)
		return nil, err
	}
	return map[string]any{"payload": append([]byte{}, buf[:n]...)}, nil
}

func (d *TCPDriver) lastErrOr(fallback string) string {
	if d.lastErr != "" {
		return d.lastErr
	}
	return fallback
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
