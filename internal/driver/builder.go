package driver

import (
	"fmt"
	"strings"

	"quantix-connector-go/internal/config"
)

func Build(protocolType string, params map[string]any, cfg config.Settings) (Driver, error) {
	switch strings.ToLower(strings.TrimSpace(protocolType)) {
	case "modbus", "modbus_tcp", "modbus_rtu":
		return NewModbusDriver(params, cfg), nil
	case "mqtt":
		return NewMqttDriver(params, cfg), nil
	case "serial":
		return NewSerialDriver(params, cfg), nil
	case "tcp":
		return NewTCPDriver(params, cfg), nil
	default:
		return nil, fmt.Errorf("unsupported protocol_type: %s", protocolType)
	}
}
