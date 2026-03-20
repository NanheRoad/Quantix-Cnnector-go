package store

import (
	"encoding/json"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func ensureSeed(db *gorm.DB) error {
	for _, t := range systemTemplates() {
		var exists ProtocolTemplate
		err := db.Where("name = ?", t.Name).First(&exists).Error
		if err == nil {
			continue
		}
		if err := db.Create(&t).Error; err != nil {
			return err
		}
	}
	return nil
}

func mustJSONMap(raw string) datatypes.JSONMap {
	out := map[string]any{}
	_ = json.Unmarshal([]byte(raw), &out)
	return datatypes.JSONMap(out)
}

func systemTemplates() []ProtocolTemplate {
	return []ProtocolTemplate{
		{
			Name:         "Std-Modbus-Scale",
			Description:  "Standard Modbus TCP scale template",
			ProtocolType: "modbus_tcp",
			IsSystem:     true,
			Template: mustJSONMap(`{
				"name": "Std-Modbus-Scale",
				"protocol_type": "modbus_tcp",
				"variables": [
					{"name":"slave_id","type":"int","default":1,"label":"Slave ID"},
					{"name":"address","type":"int","default":0,"label":"Address"}
				],
				"steps": [
					{
						"id":"read_weight",
						"name":"Read Weight",
						"trigger":"poll",
						"action":"modbus.read_input_registers",
						"params":{"slave_id":"${slave_id}","address":"${address}","count":2},
						"parse":{"type":"expression","expression":"registers[0] * 65536 + registers[1]"}
					}
				],
				"output":{"weight":"${steps.read_weight.result}","unit":"kg"}
			}`),
		},
		{
			Name:         "MQTT-Weight-Sensor",
			Description:  "MQTT push weight data",
			ProtocolType: "mqtt",
			IsSystem:     true,
			Template: mustJSONMap(`{
				"name":"MQTT-Weight-Sensor",
				"protocol_type":"mqtt",
				"variables":[{"name":"topic","type":"string","default":"sensor/weight","label":"Topic"}],
				"setup_steps":[
					{"id":"subscribe","name":"Subscribe","trigger":"setup","action":"mqtt.subscribe","params":{"topic":"${topic}","qos":1}}
				],
				"message_handler":{
					"id":"handle_message",
					"name":"Handle Message",
					"trigger":"event",
					"action":"mqtt.on_message",
					"parse":{"type":"regex","pattern":"\\\"weight\\\"\\\\s*:\\\\s*([-+]?[0-9]*\\\\.?[0-9]+)","group":1}
				},
				"output":{"weight":"${message_handler.result}","unit":"kg"}
			}`),
		},
		{
			Name:         "TSC-Serial-Print",
			Description:  "TSC serial print template",
			ProtocolType: "serial",
			IsSystem:     true,
			Template: mustJSONMap(`{
				"name":"TSC-Serial-Print",
				"protocol_type":"serial",
				"variables":[
					{"name":"print_data","type":"string","default":"SIZE 40 mm,30 mm\\nTEXT 20,20,\\\"3\\\",0,1,1,\\\"TEST\\\"\\nPRINT 1\\n","label":"Print Data"},
					{"name":"ack_timeout","type":"int","default":600,"label":"ACK Timeout ms"},
					{"name":"job_id","type":"string","default":"manual-job","label":"Job ID"}
				],
				"steps":[
					{"id":"print_send","name":"Send","trigger":"manual","action":"serial.send","params":{"data":"${print_data}"}},
					{"id":"print_ack","name":"Read ACK","trigger":"manual","action":"serial.receive","params":{"size":128,"timeout":"${ack_timeout}","delimiter":"\\n"},"parse":{"type":"regex","pattern":"OK|ACK|DONE","group":0}}
				],
				"output":{"print_ack":"${steps.print_ack.result}","job_id":"${job_id}"}
			}`),
		},
		{
			Name:         "TSC-TCP-Print",
			Description:  "TSC tcp print template",
			ProtocolType: "tcp",
			IsSystem:     true,
			Template: mustJSONMap(`{
				"name":"TSC-TCP-Print",
				"protocol_type":"tcp",
				"variables":[
					{"name":"print_data","type":"string","default":"SIZE 40 mm,30 mm\\nTEXT 20,20,\\\"3\\\",0,1,1,\\\"TEST\\\"\\nPRINT 1\\n","label":"Print Data"},
					{"name":"ack_timeout","type":"int","default":600,"label":"ACK Timeout ms"},
					{"name":"job_id","type":"string","default":"manual-job","label":"Job ID"}
				],
				"steps":[
					{"id":"print_send","name":"Send","trigger":"manual","action":"tcp.send","params":{"data":"${print_data}","wait_ack":true,"ack_timeout":"${ack_timeout}","ack_pattern":"OK|ACK|DONE"}}
				],
				"output":{"print_ack":"${steps.print_send.result.ack_ok}","job_id":"${job_id}"}
			}`),
		},
		{
			Name:         "Serial-Scanner-LineMode",
			Description:  "Serial scanner line mode",
			ProtocolType: "serial",
			IsSystem:     true,
			Template: mustJSONMap(`{
				"name":"Serial-Scanner-LineMode",
				"protocol_type":"serial",
				"variables":[
					{"name":"delimiter","type":"string","default":"\\n","label":"Delimiter"},
					{"name":"symbology","type":"string","default":"unknown","label":"Symbology"},
					{"name":"dedupe_window_ms","type":"int","default":500,"label":"Dedup Window"}
				],
				"steps":[
					{"id":"scan_line","name":"Read Line","trigger":"poll","action":"serial.receive","params":{"max_bytes":128,"timeout":300,"delimiter":"${delimiter}"},"parse":{"type":"regex","pattern":"([A-Za-z0-9_\\\\-.]+)","group":1}}
				],
				"output":{"barcode":"${steps.scan_line.result}","symbology":"${symbology}"}
			}`),
		},
		{
			Name:         "Serial-Board-Polling",
			Description:  "Serial board polling template",
			ProtocolType: "serial",
			IsSystem:     true,
			Template: mustJSONMap(`{
				"name":"Serial-Board-Polling",
				"protocol_type":"serial",
				"variables":[
					{"name":"poll_cmd","type":"string","default":"READ\\r\\n","label":"Poll Command"},
					{"name":"delimiter","type":"string","default":"\\n","label":"Delimiter"},
					{"name":"unit","type":"string","default":"kg","label":"Unit"}
				],
				"steps":[
					{"id":"send_poll","name":"Send Poll","trigger":"poll","action":"serial.send","params":{"data":"${poll_cmd}"}},
					{"id":"read_resp","name":"Read Response","trigger":"poll","action":"serial.receive","params":{"size":128,"timeout":350,"delimiter":"${delimiter}"},"parse":{"type":"regex","pattern":"([0-9.]+)","group":1}}
				],
				"output":{"board_value":"${steps.read_resp.result}","board_status":"online","alarm":false,"unit":"${unit}"}
			}`),
		},
	}
}
