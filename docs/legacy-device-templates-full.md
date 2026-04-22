# 设备模板全集

## 1. 标准 Modbus 电子台秤

```json
{
  "name": "标准 Modbus 电子台秤",
  "protocol_type": "modbus_tcp",
  "variables": [
    { "name": "slave_id", "type": "int", "default": 1, "label": "从站地址" },
    { "name": "address", "type": "int", "default": 0, "label": "寄存器地址" }
  ],
  "steps": [
    {
      "id": "read_weight",
      "name": "读取重量",
      "trigger": "poll",
      "action": "modbus.read_input_registers",
      "params": {
        "slave_id": "${slave_id}",
        "address": "${address}",
        "count": 2
      },
      "parse": {
        "type": "expression",
        "expression": "registers[0] * 65536 + registers[1]"
      }
    }
  ],
  "output": {
    "weight": "${steps.read_weight.result}",
    "unit": "kg"
  }
}

```

## 2. MQTT 重量传感器（基础）

```json
{
  "name": "MQTT 重量传感器",
  "protocol_type": "mqtt",
  "variables": [
    { "name": "topic", "type": "string", "default": "sensor/weight", "label": "主题" }
  ],
  "setup_steps": [
    {
      "id": "subscribe",
      "name": "订阅主题",
      "trigger": "setup",
      "action": "mqtt.subscribe",
      "params": { "topic": "${topic}", "qos": 1 }
    }
  ],
  "message_handler": {
    "id": "handle_message",
    "name": "处理消息",
    "trigger": "event",
    "action": "mqtt.on_message",
    "parse": {
      "type": "regex",
      "pattern": "\\\"weight\\\"\\s*:\\s*([-+]?[0-9]*\\.?[0-9]+)",
      "group": 1
    }
  },
  "output": {
    "weight": "${message_handler.result}",
    "unit": "kg"
  }
}

```

## 3. MQTT 重量传感器（支持去皮/清零）

```json
{
  "name": "MQTT 重量传感器（支持去皮清零）",
  "protocol_type": "mqtt",
  "variables": [
    { "name": "data_topic", "type": "string", "default": "sensor/weight", "label": "数据主题" },
    { "name": "cmd_topic", "type": "string", "default": "sensor/weight/cmd", "label": "控制主题" },
    { "name": "qos", "type": "int", "default": 1, "label": "QoS" }
  ],
  "setup_steps": [
    {
      "id": "subscribe_weight",
      "name": "订阅重量主题",
      "trigger": "setup",
      "action": "mqtt.subscribe",
      "params": { "topic": "${data_topic}", "qos": "${qos}" }
    }
  ],
  "steps": [
    {
      "id": "tare",
      "name": "去皮",
      "trigger": "manual",
      "action": "mqtt.publish",
      "params": {
        "topic": "${cmd_topic}",
        "payload": "{\"cmd\":\"tare\"}",
        "qos": "${qos}"
      }
    },
    {
      "id": "zero",
      "name": "清零",
      "trigger": "manual",
      "action": "mqtt.publish",
      "params": {
        "topic": "${cmd_topic}",
        "payload": "{\"cmd\":\"zero\"}",
        "qos": "${qos}"
      }
    }
  ],
  "message_handler": {
    "id": "handle_message",
    "name": "处理消息",
    "trigger": "event",
    "action": "mqtt.on_message",
    "parse": {
      "type": "regex",
      "pattern": "\\\"weight\\\"\\s*:\\s*([-+]?[0-9]*\\.?[0-9]+)",
      "group": 1
    }
  },
  "output": {
    "weight": "${message_handler.result}",
    "unit": "kg"
  }
}

```

## 4. Modbus RTU 双向交互模板

```json
{
  "name": "Modbus RTU 双向交互模板",
  "description": "轮询读取重量 + 手动去皮/清零/写入目标值",
  "protocol_type": "modbus_rtu",
  "variables": [
    { "name": "slave_id", "type": "int", "default": 1, "label": "从站地址" },
    { "name": "weight_addr", "type": "int", "default": 0, "label": "重量寄存器起始地址" },
    { "name": "status_addr", "type": "int", "default": 10, "label": "状态寄存器地址" },
    { "name": "tare_coil_addr", "type": "int", "default": 0, "label": "去皮线圈地址" },
    { "name": "zero_coil_addr", "type": "int", "default": 1, "label": "清零线圈地址" },
    { "name": "target_weight_addr", "type": "int", "default": 20, "label": "目标重量寄存器地址" },
    { "name": "scale", "type": "float", "default": 1000, "label": "重量缩放系数" }
  ],
  "steps": [
    {
      "id": "read_weight",
      "name": "读取重量",
      "trigger": "poll",
      "action": "modbus.read_input_registers",
      "params": {
        "slave_id": "${slave_id}",
        "address": "${weight_addr}",
        "count": 2
      },
      "parse": {
        "type": "expression",
        "expression": "(registers[0] * 65536 + registers[1]) / scale"
      }
    },
    {
      "id": "read_status",
      "name": "读取状态",
      "trigger": "poll",
      "action": "modbus.read_holding_registers",
      "params": {
        "slave_id": "${slave_id}",
        "address": "${status_addr}",
        "count": 1
      },
      "parse": {
        "type": "expression",
        "expression": "registers[0]"
      }
    },
    {
      "id": "tare",
      "name": "去皮",
      "trigger": "manual",
      "action": "modbus.write_coil",
      "params": {
        "slave_id": "${slave_id}",
        "address": "${tare_coil_addr}",
        "value": 1
      }
    },
    {
      "id": "zero",
      "name": "清零",
      "trigger": "manual",
      "action": "modbus.write_coil",
      "params": {
        "slave_id": "${slave_id}",
        "address": "${zero_coil_addr}",
        "value": 1
      }
    },
    {
      "id": "set_target_weight",
      "name": "写入目标重量",
      "trigger": "manual",
      "action": "modbus.write_register",
      "params": {
        "slave_id": "${slave_id}",
        "address": "${target_weight_addr}",
        "value": 1500
      }
    }
  ],
  "output": {
    "weight": "${steps.read_weight.result}",
    "status_code": "${steps.read_status.result}",
    "unit": "kg"
  }
}

```

## 5. 梅特勒托利多（Serial）

```json
{
  "name": "梅特勒托利多-天平/台秤通用模板(Serial)",
  "description": "轮询读取重量 + 手动去皮/清零，适配常见 MT-SICS/ASCII 串口设备",
  "protocol_type": "serial",
  "variables": [
    { "name": "read_command", "type": "string", "default": "SI\\r\\n", "label": "读取命令" },
    { "name": "tare_command", "type": "string", "default": "T\\r\\n", "label": "去皮命令" },
    { "name": "zero_command", "type": "string", "default": "Z\\r\\n", "label": "清零命令" },
    { "name": "receive_size", "type": "int", "default": 64, "label": "接收字节数" },
    { "name": "timeout_ms", "type": "int", "default": 1200, "label": "超时(ms)" },
    { "name": "weight_pattern", "type": "string", "default": "([-+]?[0-9]+(?:\\.[0-9]+)?)", "label": "重量正则" },
    { "name": "unit", "type": "string", "default": "kg", "label": "单位" }
  ],
  "steps": [
    {
      "id": "send_query",
      "name": "发送读取命令",
      "trigger": "poll",
      "action": "serial.send",
      "params": {
        "data": "${read_command}"
      }
    },
    {
      "id": "wait_response",
      "name": "等待响应",
      "trigger": "poll",
      "action": "delay",
      "params": {
        "milliseconds": 120
      }
    },
    {
      "id": "receive_raw",
      "name": "接收原始报文",
      "trigger": "poll",
      "action": "serial.receive",
      "params": {
        "size": "${receive_size}",
        "timeout": "${timeout_ms}"
      }
    },
    {
      "id": "parse_weight",
      "name": "提取重量",
      "trigger": "poll",
      "action": "transform.regex_extract",
      "params": {
        "input": "${steps.receive_raw.result.payload}",
        "pattern": "${weight_pattern}",
        "group": 1
      },
      "parse": {
        "type": "expression",
        "expression": "float(payload)"
      }
    },
    {
      "id": "tare",
      "name": "去皮",
      "trigger": "manual",
      "action": "serial.send",
      "params": {
        "data": "${tare_command}"
      }
    },
    {
      "id": "zero",
      "name": "清零",
      "trigger": "manual",
      "action": "serial.send",
      "params": {
        "data": "${zero_command}"
      }
    }
  ],
  "output": {
    "weight": "${steps.parse_weight.result}",
    "unit": "${unit}"
  }
}

```

## 6. 奥豪斯 Navigator（Serial）

```json
{
  "name": "奥豪斯 Navigator 天平 (NV/NVL/NVT)",
  "description": "适配奥豪斯 Navigator 系列电子天平，支持轮询读取、打印、去皮、清零、切换单位/模式",
  "protocol_type": "serial",
  "variables": [
    { "name": "poll_command", "type": "string", "default": "P\\r", "label": "轮询读取命令" },
    { "name": "print_stable_command", "type": "string", "default": "SP\\r", "label": "打印稳定值命令" },
    { "name": "print_current_command", "type": "string", "default": "IP\\r", "label": "打印当前显示命令" },
    { "name": "tare_command", "type": "string", "default": "T\\r", "label": "去皮命令" },
    { "name": "zero_command", "type": "string", "default": "Z\\r", "label": "清零命令" },
    { "name": "unit_command", "type": "string", "default": "U\\r", "label": "切换单位命令" },
    { "name": "mode_command", "type": "string", "default": "M\\r", "label": "切换模式命令" },
    { "name": "receive_size", "type": "int", "default": 64, "label": "接收字节数" },
    { "name": "timeout_ms", "type": "int", "default": 1200, "label": "超时(ms)" },
    { "name": "weight_pattern", "type": "string", "default": "\\s*([-+]?[0-9]+(?:\\.[0-9]+)?)\\s*([a-zA-Z]+)", "label": "重量和单位正则" },
    { "name": "default_unit", "type": "string", "default": "g", "label": "默认单位" }
  ],
  "steps": [
    {
      "id": "send_poll",
      "name": "发送轮询命令",
      "trigger": "poll",
      "action": "serial.send",
      "params": {
        "data": "${poll_command}"
      }
    },
    {
      "id": "wait_poll_response",
      "name": "等待响应",
      "trigger": "poll",
      "action": "delay",
      "params": {
        "milliseconds": 150
      }
    },
    {
      "id": "receive_poll_raw",
      "name": "接收轮询响应",
      "trigger": "poll",
      "action": "serial.receive",
      "params": {
        "size": "${receive_size}",
        "timeout": "${timeout_ms}"
      }
    },
    {
      "id": "parse_weight",
      "name": "解析重量和单位",
      "trigger": "poll",
      "action": "transform.regex_extract",
      "params": {
        "input": "${steps.receive_poll_raw.result.payload}",
        "pattern": "${weight_pattern}",
        "group": 1
      },
      "parse": {
        "type": "expression",
        "expression": "float(payload)"
      }
    },
    {
      "id": "parse_unit",
      "name": "解析单位",
      "trigger": "poll",
      "action": "transform.regex_extract",
      "params": {
        "input": "${steps.receive_poll_raw.result.payload}",
        "pattern": "${weight_pattern}",
        "group": 2
      }
    },
    {
      "id": "print_stable",
      "name": "打印稳定值",
      "trigger": "manual",
      "action": "serial.send",
      "params": {
        "data": "${print_stable_command}"
      }
    },
    {
      "id": "print_current",
      "name": "打印当前显示",
      "trigger": "manual",
      "action": "serial.send",
      "params": {
        "data": "${print_current_command}"
      }
    },
    {
      "id": "tare",
      "name": "去皮",
      "trigger": "manual",
      "action": "serial.send",
      "params": {
        "data": "${tare_command}"
      }
    },
    {
      "id": "zero",
      "name": "清零",
      "trigger": "manual",
      "action": "serial.send",
      "params": {
        "data": "${zero_command}"
      }
    },
    {
      "id": "toggle_unit",
      "name": "切换单位",
      "trigger": "manual",
      "action": "serial.send",
      "params": {
        "data": "${unit_command}"
      }
    },
    {
      "id": "toggle_mode",
      "name": "切换模式",
      "trigger": "manual",
      "action": "serial.send",
      "params": {
        "data": "${mode_command}"
      }
    }
  ],
  "output": {
    "weight": "${steps.parse_weight.result}",
    "unit": "${steps.parse_unit.result}",
    "raw_payload": "${steps.receive_poll_raw.result.payload}"
  }
}

```

## 7. TSC 串口打印

```json
{
  "name": "TSC-Serial-Print",
  "protocol_type": "serial",
  "variables": [
    { "name": "print_data", "type": "string", "default": "SIZE 40 mm,30 mm\\nTEXT 20,20,\"3\",0,1,1,\"TEST\"\\nPRINT 1\\n", "label": "打印指令" },
    { "name": "ack_timeout", "type": "int", "default": 600, "label": "ACK超时(ms)" },
    { "name": "job_id", "type": "string", "default": "manual-job", "label": "任务ID" }
  ],
  "steps": [
    {
      "id": "print_send",
      "name": "发送打印指令",
      "trigger": "manual",
      "action": "serial.send",
      "params": { "data": "${print_data}" }
    },
    {
      "id": "print_ack",
      "name": "读取ACK",
      "trigger": "manual",
      "action": "serial.receive",
      "params": { "size": 128, "timeout": "${ack_timeout}", "delimiter": "\\n" },
      "parse": { "type": "regex", "pattern": "OK|ACK|DONE", "group": 0 }
    }
  ],
  "output": {
    "print_ack": "${steps.print_ack.result}",
    "job_id": "${job_id}"
  }
}

```

## 8. TSC TCP 打印

```json
{
  "name": "TSC-TCP-Print",
  "protocol_type": "tcp",
  "variables": [
    { "name": "print_data", "type": "string", "default": "SIZE 40 mm,30 mm\\nTEXT 20,20,\"3\",0,1,1,\"TEST\"\\nPRINT 1\\n", "label": "打印指令" },
    { "name": "ack_timeout", "type": "int", "default": 600, "label": "ACK超时(ms)" },
    { "name": "job_id", "type": "string", "default": "manual-job", "label": "任务ID" }
  ],
  "steps": [
    {
      "id": "print_send",
      "name": "发送打印指令",
      "trigger": "manual",
      "action": "tcp.send",
      "params": {
        "data": "${print_data}",
        "wait_ack": true,
        "ack_timeout": "${ack_timeout}",
        "ack_pattern": "OK|ACK|DONE"
      }
    }
  ],
  "output": {
    "print_ack": "${steps.print_send.result.ack_ok}",
    "job_id": "${job_id}"
  }
}

```

## 9. 串口扫码枪行模式

```json
{
  "name": "Serial-Scanner-LineMode",
  "protocol_type": "serial",
  "variables": [
    { "name": "delimiter", "type": "string", "default": "\\n", "label": "拆帧分隔符" },
    { "name": "symbology", "type": "string", "default": "unknown", "label": "码制" },
    { "name": "dedupe_window_ms", "type": "int", "default": 500, "label": "去重窗口(ms)" }
  ],
  "steps": [
    {
      "id": "scan_line",
      "name": "读取扫码行",
      "trigger": "poll",
      "action": "serial.receive",
      "params": { "max_bytes": 128, "timeout": 300, "delimiter": "${delimiter}" },
      "parse": { "type": "regex", "pattern": "([A-Za-z0-9_\\\\-\\\\.]+)", "group": 1 }
    }
  ],
  "output": {
    "barcode": "${steps.scan_line.result}",
    "symbology": "${symbology}"
  }
}

```

## 10. 串口看板轮询

```json
{
  "name": "Serial-Board-Polling",
  "protocol_type": "serial",
  "variables": [
    { "name": "poll_cmd", "type": "string", "default": "READ\\r\\n", "label": "轮询命令" },
    { "name": "delimiter", "type": "string", "default": "\\n", "label": "分隔符" },
    { "name": "unit", "type": "string", "default": "kg", "label": "单位" }
  ],
  "steps": [
    {
      "id": "send_poll",
      "name": "发送轮询命令",
      "trigger": "poll",
      "action": "serial.send",
      "params": { "data": "${poll_cmd}" }
    },
    {
      "id": "read_value",
      "name": "读取看板响应",
      "trigger": "poll",
      "action": "serial.receive",
      "params": { "max_bytes": 128, "timeout": 500, "delimiter": "${delimiter}" },
      "parse": { "type": "regex", "pattern": "[-+]?[0-9]*\\.?[0-9]+", "group": 0 }
    }
  ],
  "output": {
    "board_value": "${steps.read_value.result}",
    "board_status": "online",
    "alarm": false,
    "unit": "${unit}"
  }
}

```

## 11. Modbus TCP Test Server 双向模板

```json
{
  "name": "Modbus TCP Test Server 双向模板",
  "protocol_type": "modbus_tcp",
  "variables": [
    { "name": "slave_id", "type": "int", "default": 1, "label": "从站地址" },
    { "name": "address", "type": "int", "default": 0, "label": "重量寄存器起始地址" },
    { "name": "scale", "type": "float", "default": 1000, "label": "重量缩放系数" },
    { "name": "unit", "type": "string", "default": "kg", "label": "重量单位" },
    { "name": "tare_control_addr", "type": "int", "default": 100, "label": "去皮控制地址" },
    { "name": "zero_control_addr", "type": "int", "default": 101, "label": "清零控制地址" }
  ],
  "steps": [
    {
      "id": "read_weight",
      "name": "读取净重(FC4)",
      "trigger": "poll",
      "action": "modbus.read_input_registers",
      "params": {
        "slave_id": "${slave_id}",
        "address": "${address}",
        "count": 2
      },
      "parse": {
        "type": "expression",
        "expression": "((registers[0] * 65536 + registers[1]) - ((registers[0] * 65536 + registers[1]) >= 2147483648 ? 4294967296 : 0)) / scale"
      }
    },
    {
      "id": "tare",
      "name": "去皮",
      "trigger": "manual",
      "action": "modbus.write_register",
      "params": {
        "slave_id": "${slave_id}",
        "address": "${tare_control_addr}",
        "value": 1
      }
    },
    {
      "id": "zero",
      "name": "清零",
      "trigger": "manual",
      "action": "modbus.write_register",
      "params": {
        "slave_id": "${slave_id}",
        "address": "${zero_control_addr}",
        "value": 1
      }
    }
  ],
  "output": {
    "weight": "${steps.read_weight.result}",
    "unit": "${unit}"
  }
}

```

## 12. 虚拟串口 RTU 单寄存器模板

```json
{
  "name": "测试设备-Modbus RTU 虚拟串口",
  "protocol_type": "modbus_rtu",
  "variables": [
    { "name": "slave_id", "type": "int", "default": 1, "label": "从站地址" },
    { "name": "weight_addr", "type": "int", "default": 0, "label": "重量寄存器起始地址" },
    { "name": "scale", "type": "float", "default": 100, "label": "重量缩放系数" }
  ],
  "steps": [
    {
      "id": "read_weight",
      "name": "读取重量",
      "trigger": "poll",
      "action": "modbus.read_holding_registers",
      "params": {
        "slave_id": "${slave_id}",
        "address": "${weight_addr}",
        "count": 1
      },
      "parse": {
        "type": "expression",
        "expression": "registers[0] / scale"
      }
    }
  ],
  "output": {
    "weight": "${steps.read_weight.result}",
    "unit": "kg"
  }
}

```

## 13. 虚拟串口 RTU 双寄存器模板

```json
{
  "name": "测试设备-Modbus RTU 双寄存器重量",
  "protocol_type": "modbus_rtu",
  "variables": [
    { "name": "slave_id", "type": "int", "default": 1, "label": "从站地址" },
    { "name": "weight_addr", "type": "int", "default": 0, "label": "重量寄存器起始地址" },
    { "name": "scale", "type": "float", "default": 1000, "label": "重量缩放系数" }
  ],
  "steps": [
    {
      "id": "read_weight",
      "name": "读取重量",
      "trigger": "poll",
      "action": "modbus.read_holding_registers",
      "params": {
        "slave_id": "${slave_id}",
        "address": "${weight_addr}",
        "count": 2
      },
      "parse": {
        "type": "expression",
        "expression": "(registers[0] * 65536 + registers[1]) / scale"
      }
    }
  ],
  "output": {
    "weight": "${steps.read_weight.result}",
    "unit": "kg"
  }
}

```

## 兼容性提示（当前 Go 执行器）

- 第 6 个模板已移除 `payload.strip()`，直接使用正则分组提取单位。
- 第 11 个模板已改为 `expr` 兼容的 `?:` 条件表达式。

## 14. Modbus TCP 调试专用模板（新增）

说明：该模板偏“联调读写”用途，重量解析为通用 `寄存器值 / scale`。若你需要和 `modbus_tcp_test_server.py` 严格对齐（有符号 32 位 + `/1000` + 可配小数位/单位），请使用第 15 节模板。

```json
{
  "name": "Modbus TCP 调试专用模板",
  "description": "用于联调阶段：轮询读取 + 手工读写寄存器/线圈",
  "protocol_type": "modbus_tcp",
  "variables": [
    { "name": "slave_id", "type": "int", "default": 1, "label": "从站地址" },
    { "name": "read_addr", "type": "int", "default": 0, "label": "读取起始地址" },
    { "name": "read_count", "type": "int", "default": 2, "label": "读取数量" },
    { "name": "scale", "type": "float", "default": 1000, "label": "重量缩放系数" },
    { "name": "write_register_addr", "type": "int", "default": 100, "label": "写寄存器地址" },
    { "name": "write_register_value", "type": "int", "default": 1, "label": "写寄存器值" },
    { "name": "write_coil_addr", "type": "int", "default": 101, "label": "写线圈地址" },
    { "name": "write_coil_value", "type": "bool", "default": true, "label": "写线圈值" }
  ],
  "steps": [
    {
      "id": "poll_read_input",
      "name": "轮询读输入寄存器",
      "trigger": "poll",
      "action": "modbus.read_input_registers",
      "params": {
        "slave_id": "${slave_id}",
        "address": "${read_addr}",
        "count": "${read_count}"
      }
    },
    {
      "id": "poll_weight",
      "name": "轮询重量解析",
      "trigger": "poll",
      "action": "modbus.read_input_registers",
      "params": {
        "slave_id": "${slave_id}",
        "address": "${read_addr}",
        "count": 2
      },
      "parse": {
        "type": "expression",
        "expression": "(registers[0] * 65536 + registers[1]) / scale"
      }
    },
    {
      "id": "dbg_read_input",
      "name": "手工读输入寄存器",
      "trigger": "manual",
      "action": "modbus.read_input_registers",
      "params": {
        "slave_id": "${slave_id}",
        "address": "${read_addr}",
        "count": "${read_count}"
      }
    },
    {
      "id": "dbg_read_holding",
      "name": "手工读保持寄存器",
      "trigger": "manual",
      "action": "modbus.read_holding_registers",
      "params": {
        "slave_id": "${slave_id}",
        "address": "${read_addr}",
        "count": "${read_count}"
      }
    },
    {
      "id": "dbg_write_register",
      "name": "手工写单寄存器",
      "trigger": "manual",
      "action": "modbus.write_register",
      "params": {
        "slave_id": "${slave_id}",
        "address": "${write_register_addr}",
        "value": "${write_register_value}"
      }
    },
    {
      "id": "dbg_write_coil",
      "name": "手工写单线圈",
      "trigger": "manual",
      "action": "modbus.write_coil",
      "params": {
        "slave_id": "${slave_id}",
        "address": "${write_coil_addr}",
        "value": "${write_coil_value}"
      }
    }
  ],
  "output": {
    "weight": "${steps.poll_weight.result}",
    "unit": "kg",
    "poll_registers": "${steps.poll_read_input.result.registers}"
  }
}
```

## 15. Modbus TCP（/1000 + 小数位可配置）模板（新增）

```json
{
  "name": "Modbus TCP 测试（/1000 可配小数位）",
  "description": "读取2个输入寄存器组成32位有符号值，按 scale 缩放；单位与小数位可配置",
  "protocol_type": "modbus_tcp",
  "variables": [
    { "name": "slave_id", "type": "int", "default": 1, "label": "从站地址" },
    { "name": "address", "type": "int", "default": 0, "label": "重量寄存器起始地址" },
    { "name": "scale", "type": "float", "default": 1000, "label": "缩放系数（用于除法）" },
    { "name": "unit", "type": "string", "default": "kg", "label": "显示单位" },
    { "name": "decimals", "type": "int", "default": 2, "label": "显示小数位" },
    { "name": "tare_control_addr", "type": "int", "default": 100, "label": "去皮控制地址" },
    { "name": "zero_control_addr", "type": "int", "default": 101, "label": "清零控制地址" }
  ],
  "steps": [
    {
      "id": "read_weight",
      "name": "读取净重(FC4)",
      "trigger": "poll",
      "action": "modbus.read_input_registers",
      "params": {
        "slave_id": "${slave_id}",
        "address": "${address}",
        "count": 2
      },
      "parse": {
        "type": "expression",
        "expression": "((registers[0] * 65536 + registers[1]) - ((registers[0] * 65536 + registers[1]) >= 2147483648 ? 4294967296 : 0)) / scale"
      }
    },
    {
      "id": "tare",
      "name": "去皮",
      "trigger": "manual",
      "action": "modbus.write_register",
      "params": {
        "slave_id": "${slave_id}",
        "address": "${tare_control_addr}",
        "value": 1
      }
    },
    {
      "id": "zero",
      "name": "清零",
      "trigger": "manual",
      "action": "modbus.write_register",
      "params": {
        "slave_id": "${slave_id}",
        "address": "${zero_control_addr}",
        "value": 1
      }
    }
  ],
  "output": {
    "weight": "${steps.read_weight.result}",
    "unit": "${unit}",
    "decimals": "${decimals}"
  }
}
```

## 16. 梅特勒托利多 MT-SICS TCP 低延迟模板（新增）

### 16.1 场景说明

本节用于梅特勒托利多台秤/天平通过 MT-SICS 协议、TCP Server 模式接入 Quantix Connector 的场景。

本次现场已验证联通参数：

- 设备协议：`MT-SICS`
- 传输方式：`TCP`
- 设备地址：`192.168.3.22`
- 端口：`9761`

目标：

- 低延迟读取当前重量
- 保留手动去皮/清零能力
- 避免串口调试助手中常见的乱码、半包、粘包误判

### 16.2 为什么选 `SIU`

本模板默认读取命令使用：

```text
SIU\r\n
```

原因：

- `SIU`：立即返回当前单位重量
- `S`：等待稳定值，延迟更高
- `SI`：也可立即返回，但当前场景明确要求按当前单位读取，因此优先 `SIU`
- `SIR` / `SR`：更适合连续流式输出，不适合当前执行器的请求-响应式轮询模型

结论：

- 如果目标是“响应速度优先、延迟尽量低”，推荐 `SIU`
- 如果后续业务改成“只关心稳定值”，再切换 `S`

### 16.3 推荐创建设备方式

协议模板建议：

- 模板名称：`MT-SICS-TCP-Scale`
- 协议类型：`tcp`
- 设备分类：`weight`

连接参数：

```json
{
  "host": "192.168.3.22",
  "port": 9761
}
```

模板变量：

```json
{
  "read_command": "SIU\r\n",
  "tare_command": "T\r\n",
  "zero_command": "Z\r\n",
  "receive_size": 64,
  "timeout_ms": 80,
  "line_pattern": "^[A-Z]{1,3}\\s+([A-Z])\\s+([-+]?[0-9]+(?:\\.[0-9]+)?)\\s+([A-Za-z]+)"
}
```

设备运行建议：

```json
{
  "device_category": "weight",
  "poll_interval": 0.05,
  "enabled": true
}
```

说明：

- `poll_interval=0.05` 表示 50ms 轮询一次
- 若现场网络稳定、秤响应很快，可压到 `0.03`
- 若现场存在偶发超时，可回调到 `0.08 ~ 0.10`

### 16.4 模板变量解释

- `read_command`
  - 默认 `SIU\r\n`
  - 用于轮询读取当前单位重量
- `tare_command`
  - 默认 `T\r\n`
  - 手动去皮
- `zero_command`
  - 默认 `Z\r\n`
  - 手动清零
- `receive_size`
  - 默认 `64`
  - 单次读取响应的缓冲区大小
  - MT-SICS 文本响应通常较短，64 足够
- `timeout_ms`
  - 默认 `80`
  - 单次 TCP 读取超时
  - 低延迟场景建议从 80ms 开始
- `line_pattern`
  - 默认：
    `^[A-Z]{1,3}\s+([A-Z])\s+([-+]?[0-9]+(?:\.[0-9]+)?)\s+([A-Za-z]+)`
  - 用于提取：
    - 第 1 组：状态位
    - 第 2 组：重量值
    - 第 3 组：单位

### 16.5 响应格式说明

根据 MT-SICS 文档，典型返回示例类似：

```text
S S 0.256 g
```

含义可理解为：

- 第 1 列：命令/响应前缀
- 第 2 列：状态位
- 第 3 列：重量值
- 第 4 列：单位

本模板提取策略：

- `parse_status` 提取状态位
- `parse_weight` 提取重量数值并转为数值类型
- `parse_unit` 提取单位

### 16.6 输出字段说明

模板最终输出：

```json
"output": {
  "weight": "${steps.parse_weight.result}",
  "unit": "${steps.parse_unit.result}",
  "decimals": 3,
  "stability": "${steps.parse_status.result}",
  "raw_payload": "${steps.read_resp.result.payload}",
  "transport": "tcp",
  "protocol": "mt-sics"
}
```

字段说明：

- `weight`
  - 当前解析出的重量值
- `unit`
  - 当前单位，例如 `g`、`kg`
- `decimals`
  - 固定为 `3`
  - 用于前端把 `0` 显示成 `0.000`
- `stability`
  - 当前响应中的状态位
- `raw_payload`
  - 原始响应文本
  - 联调排错时非常有用
- `transport`
  - 固定 `tcp`
- `protocol`
  - 固定 `mt-sics`

### 16.7 低延迟调优建议

推荐起始配置：

- `poll_interval = 0.05`
- `timeout_ms = 80`
- `receive_size = 64`

如果你要继续压低延迟，建议按这个顺序试：

1. 保持 `timeout_ms=80`，把 `poll_interval` 从 `0.05` 压到 `0.03`
2. 如果出现偶发超时，把 `timeout_ms` 提到 `100` 或 `120`
3. 如果现场网络抖动明显，把 `poll_interval` 回调到 `0.08`

不建议的做法：

- 每次连接都发送 `@`
  - 会清除皮重并重置设备状态
- 默认改成 `S`
  - 会等待稳定，读数延迟上升
- 使用持续输出命令去适配当前轮询引擎
  - 容易造成报文边界管理复杂化

### 16.8 关于乱码

如果你在串口调试助手中偶尔看到乱码，通常不一定是设备真的发错了，而更常见是：

- 显示编码不对
- CR/LF 行结束符处理不一致
- 半包读取
- 粘包显示
- 串口参数不匹配

本模板采用 TCP ASCII 文本方式，处理逻辑更稳定：

- 发送 ASCII 查询命令
- 读取一段短文本响应
- 用正则提取状态、重量、单位
- 保留 `raw_payload` 供联调排查

### 16.9 常见异常与排查

如果出现连接异常，优先检查：

1. 是否误用了 `modbus_tcp` 模板
2. 协议模板 `protocol_type` 是否为 `tcp`
3. 连接参数是否为：
   - `host=192.168.3.22`
   - `port=9761`
4. 是否把整个“导入包 JSON”粘进了协议编辑器，而不是模板本体

典型错误：

```text
stage=poll; protocol=modbus_tcp; endpoint=192.168.3.22:9761; error=modbus: length in response header ...
```

这通常表示：

- 设备本身没问题
- 但运行时驱动仍然按 Modbus TCP 在解析 MT-SICS 文本

### 16.10 可直接使用的模板

```json
{
  "name": "MT-SICS-TCP-Scale",
  "description": "Mettler Toledo SICS scale over TCP; optimized for low-latency polling with SIU command",
  "protocol_type": "tcp",
  "variables": [
    { "name": "read_command", "type": "string", "default": "SIU\r\n", "label": "Read Command" },
    { "name": "tare_command", "type": "string", "default": "T\r\n", "label": "Tare Command" },
    { "name": "zero_command", "type": "string", "default": "Z\r\n", "label": "Zero Command" },
    { "name": "receive_size", "type": "int", "default": 64, "label": "Receive Size" },
    { "name": "timeout_ms", "type": "int", "default": 80, "label": "Timeout ms" },
    {
      "name": "line_pattern",
      "type": "string",
      "default": "^[A-Z]{1,3}\\s+([A-Z])\\s+([-+]?[0-9]+(?:\\.[0-9]+)?)\\s+([A-Za-z]+)",
      "label": "SICS Line Pattern"
    }
  ],
  "steps": [
    {
      "id": "send_read",
      "name": "Send SIU",
      "trigger": "poll",
      "action": "tcp.send",
      "params": {
        "data": "${read_command}",
        "encoding": "ascii"
      }
    },
    {
      "id": "read_resp",
      "name": "Read SICS Response",
      "trigger": "poll",
      "action": "tcp.receive",
      "params": {
        "size": "${receive_size}",
        "timeout": "${timeout_ms}"
      }
    },
    {
      "id": "parse_status",
      "name": "Parse Status",
      "trigger": "poll",
      "action": "transform.regex_extract",
      "params": {
        "input": "${steps.read_resp.result.payload}",
        "pattern": "${line_pattern}",
        "group": 1
      }
    },
    {
      "id": "parse_weight",
      "name": "Parse Weight",
      "trigger": "poll",
      "action": "transform.regex_extract",
      "params": {
        "input": "${steps.read_resp.result.payload}",
        "pattern": "${line_pattern}",
        "group": 2
      },
      "parse": {
        "type": "expression",
        "expression": "float(payload)"
      }
    },
    {
      "id": "parse_unit",
      "name": "Parse Unit",
      "trigger": "poll",
      "action": "transform.regex_extract",
      "params": {
        "input": "${steps.read_resp.result.payload}",
        "pattern": "${line_pattern}",
        "group": 3
      }
    },
    {
      "id": "tare",
      "name": "Tare",
      "trigger": "manual",
      "action": "tcp.send",
      "params": {
        "data": "${tare_command}",
        "encoding": "ascii",
        "wait_ack": true,
        "ack_timeout": "${timeout_ms}",
        "ack_size": "${receive_size}",
        "ack_pattern": "^[A-Z]{1,3}.*"
      }
    },
    {
      "id": "zero",
      "name": "Zero",
      "trigger": "manual",
      "action": "tcp.send",
      "params": {
        "data": "${zero_command}",
        "encoding": "ascii",
        "wait_ack": true,
        "ack_timeout": "${timeout_ms}",
        "ack_size": "${receive_size}",
        "ack_pattern": "^[A-Z]{1,3}.*"
      }
    }
  ],
  "output": {
    "weight": "${steps.parse_weight.result}",
    "unit": "${steps.parse_unit.result}",
    "decimals": 3,
    "stability": "${steps.parse_status.result}",
    "raw_payload": "${steps.read_resp.result.payload}",
    "transport": "tcp",
    "protocol": "mt-sics"
  }
}
```
