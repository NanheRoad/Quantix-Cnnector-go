package driver

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"quantix-connector-go/internal/config"

	"github.com/goburrow/modbus"
)

type ModbusDriver struct {
	cfg       config.Settings
	params    map[string]any
	handler   *modbus.TCPClientHandler
	rtu       *modbus.RTUClientHandler
	client    modbus.Client
	connected bool
	lastErr   string
	mu        sync.Mutex
}

func NewModbusDriver(params map[string]any, cfg config.Settings) *ModbusDriver {
	return &ModbusDriver{params: params, cfg: cfg}
}

func (d *ModbusDriver) Connect(ctx context.Context) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastErr = ""
	d.closeUnlocked()
	host := asString(d.params, "host", "")
	port := asInt(d.params, "port", 502)
	if host != "" {
		h := modbus.NewTCPClientHandler(fmt.Sprintf("%s:%d", host, port))
		h.Timeout = 2 * time.Second
		if err := h.Connect(); err != nil {
			d.lastErr = fmt.Sprintf("modbus tcp connect failed: %v", err)
			if d.cfg.SimulateOnConnectFail {
				d.connected = true
				d.client = nil
				return true, nil
			}
			d.connected = false
			return false, nil
		}
		d.handler = h
		d.client = modbus.NewClient(h)
		d.connected = true
		_ = ctx
		return true, nil
	}
	portName := asString(d.params, "port", "")
	if portName != "" {
		h := modbus.NewRTUClientHandler(portName)
		h.BaudRate = asInt(d.params, "baudrate", 9600)
		h.DataBits = asInt(d.params, "bytesize", 8)
		h.Parity = asString(d.params, "parity", "N")
		h.StopBits = asInt(d.params, "stopbits", 1)
		h.Timeout = time.Duration(asFloat(d.params, "timeout", 1.0) * float64(time.Second))
		if err := h.Connect(); err != nil {
			d.lastErr = fmt.Sprintf("modbus rtu connect failed: %v", err)
			if d.cfg.SimulateOnConnectFail {
				d.connected = true
				d.client = nil
				return true, nil
			}
			d.connected = false
			return false, nil
		}
		d.rtu = h
		d.client = modbus.NewClient(h)
		d.connected = true
		_ = ctx
		return true, nil
	}
	d.lastErr = "modbus requires host:port or serial port"
	if d.cfg.SimulateOnConnectFail {
		d.connected = true
		return true, nil
	}
	return false, nil
}

func (d *ModbusDriver) Disconnect(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closeUnlocked()
	d.connected = false
	_ = ctx
	return nil
}

func (d *ModbusDriver) IsConnected() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.connected
}

func (d *ModbusDriver) LastError() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lastErr
}

func (d *ModbusDriver) RegisterMessageHandler(handler MessageHandler) {
	_ = handler
}

func (d *ModbusDriver) ExecuteAction(ctx context.Context, action string, params map[string]any) (any, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.client == nil && d.connected {
		return d.simulate(action, params), nil
	}
	if d.client == nil {
		return nil, fmt.Errorf(d.lastErrOr("modbus client is not connected"))
	}
	slaveID := byte(asInt(params, "slave_id", 1))
	if d.handler != nil {
		d.handler.SlaveId = slaveID
	}
	if d.rtu != nil {
		d.rtu.SlaveId = slaveID
	}
	address := uint16(asInt(params, "address", 0))
	count := uint16(asInt(params, "count", 2))
	value := uint16(asInt(params, "value", 0))

	switch action {
	case "modbus.read_input_registers":
		b, err := d.client.ReadInputRegisters(address, count)
		if err != nil {
			d.markDisconnectedUnlocked(err)
			return nil, err
		}
		return map[string]any{"registers": bytesToU16(b)}, nil
	case "modbus.read_holding_registers":
		b, err := d.client.ReadHoldingRegisters(address, count)
		if err != nil {
			d.markDisconnectedUnlocked(err)
			return nil, err
		}
		return map[string]any{"registers": bytesToU16(b)}, nil
	case "modbus.read_coils":
		b, err := d.client.ReadCoils(address, count)
		if err != nil {
			d.markDisconnectedUnlocked(err)
			return nil, err
		}
		return map[string]any{"coils": bytesToBits(b, int(count))}, nil
	case "modbus.read_discrete_inputs":
		b, err := d.client.ReadDiscreteInputs(address, count)
		if err != nil {
			d.markDisconnectedUnlocked(err)
			return nil, err
		}
		return map[string]any{"coils": bytesToBits(b, int(count))}, nil
	case "modbus.write_register":
		_, err := d.client.WriteSingleRegister(address, value)
		if err != nil {
			d.markDisconnectedUnlocked(err)
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	case "modbus.write_coil":
		var coil uint16
		if asBool(params, "value", false) {
			coil = 0xFF00
		}
		_, err := d.client.WriteSingleCoil(address, coil)
		if err != nil {
			d.markDisconnectedUnlocked(err)
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	default:
		_ = ctx
		return nil, fmt.Errorf("unsupported action for ModbusDriver: %s", action)
	}
}

func (d *ModbusDriver) simulate(action string, params map[string]any) map[string]any {
	if len(action) >= 11 && action[:11] == "modbus.read" {
		count := asInt(params, "count", 2)
		if count < 2 {
			count = 2
		}
		kg := rand.Float64() * 30.0
		raw := int(kg * 1000)
		hi := (raw >> 16) & 0xFFFF
		lo := raw & 0xFFFF
		out := make([]int, 0, count)
		out = append(out, hi, lo)
		for len(out) < count {
			out = append(out, 0)
		}
		return map[string]any{"registers": out[:count], "coils": []bool{true, false, true, false}}
	}
	if len(action) >= 12 && action[:12] == "modbus.write" {
		return map[string]any{"ok": true}
	}
	return map[string]any{}
}

func (d *ModbusDriver) lastErrOr(fallback string) string {
	if d.lastErr != "" {
		return d.lastErr
	}
	return fallback
}

func (d *ModbusDriver) markDisconnectedUnlocked(err error) {
	d.closeUnlocked()
	d.connected = false
	if err != nil {
		d.lastErr = err.Error()
	}
}

func (d *ModbusDriver) closeUnlocked() {
	if d.handler != nil {
		_ = d.handler.Close()
	}
	if d.rtu != nil {
		_ = d.rtu.Close()
	}
	d.handler = nil
	d.rtu = nil
	d.client = nil
}

func bytesToU16(in []byte) []int {
	out := make([]int, 0, len(in)/2)
	for i := 0; i+1 < len(in); i += 2 {
		out = append(out, int(uint16(in[i])<<8|uint16(in[i+1])))
	}
	return out
}

func bytesToBits(in []byte, count int) []bool {
	out := make([]bool, 0, count)
	for i := 0; i < count; i++ {
		byteIdx := i / 8
		if byteIdx >= len(in) {
			out = append(out, false)
			continue
		}
		b := in[byteIdx]
		mask := byte(1 << uint(i%8))
		out = append(out, b&mask != 0)
	}
	return out
}
