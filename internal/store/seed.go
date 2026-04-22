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
			Name:         "MT-SICS-TCP-Scale",
			Description:  "Mettler Toledo SICS scale over TCP",
			ProtocolType: "tcp",
			IsSystem:     true,
			Template: mustJSONMap(`{
				"name":"MT-SICS-TCP-Scale",
				"description":"Mettler Toledo SICS scale over TCP; optimized for low-latency polling with SIU command",
				"protocol_type":"tcp",
				"variables":[
					{"name":"read_command","type":"string","default":"SIU\\r\\n","label":"Read Command"},
					{"name":"tare_command","type":"string","default":"T\\r\\n","label":"Tare Command"},
					{"name":"zero_command","type":"string","default":"Z\\r\\n","label":"Zero Command"},
					{"name":"receive_size","type":"int","default":64,"label":"Receive Size"},
					{"name":"timeout_ms","type":"int","default":80,"label":"Timeout ms"},
					{"name":"line_pattern","type":"string","default":"^[A-Z]{1,3}\\s+([A-Z])\\s+([-+]?[0-9]+(?:\\\\.[0-9]+)?)\\s+([A-Za-z]+)","label":"SICS Line Pattern"}
				],
				"steps":[
					{"id":"send_read","name":"Send SIU","trigger":"poll","action":"tcp.send","params":{"data":"${read_command}","encoding":"ascii"}},
					{"id":"read_resp","name":"Read SICS Response","trigger":"poll","action":"tcp.receive","params":{"size":"${receive_size}","timeout":"${timeout_ms}"}},
					{"id":"parse_status","name":"Parse Status","trigger":"poll","action":"transform.regex_extract","params":{"input":"${steps.read_resp.result.payload}","pattern":"${line_pattern}","group":1}},
					{"id":"parse_weight","name":"Parse Weight","trigger":"poll","action":"transform.regex_extract","params":{"input":"${steps.read_resp.result.payload}","pattern":"${line_pattern}","group":2},"parse":{"type":"expression","expression":"float(payload)"}},
					{"id":"parse_unit","name":"Parse Unit","trigger":"poll","action":"transform.regex_extract","params":{"input":"${steps.read_resp.result.payload}","pattern":"${line_pattern}","group":3}},
					{"id":"tare","name":"Tare","trigger":"manual","action":"tcp.send","params":{"data":"${tare_command}","encoding":"ascii","wait_ack":true,"ack_timeout":"${timeout_ms}","ack_size":"${receive_size}","ack_pattern":"^[A-Z]{1,3}.*"}},
					{"id":"zero","name":"Zero","trigger":"manual","action":"tcp.send","params":{"data":"${zero_command}","encoding":"ascii","wait_ack":true,"ack_timeout":"${timeout_ms}","ack_size":"${receive_size}","ack_pattern":"^[A-Z]{1,3}.*"}}
				],
				"output":{
					"weight":"${steps.parse_weight.result}",
					"unit":"${steps.parse_unit.result}",
					"stability":"${steps.parse_status.result}",
					"raw_payload":"${steps.read_resp.result.payload}",
					"transport":"tcp",
					"protocol":"mt-sics"
				}
			}`),
		},
	}
}
