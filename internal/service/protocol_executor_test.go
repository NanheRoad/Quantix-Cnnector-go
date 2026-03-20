package service

import "testing"

func TestParseStructResult(t *testing.T) {
	raw := map[string]any{
		"payload": []byte{0x00, 0x64, 0x01, 0x02, 'A', 'B'},
	}
	parseCfg := map[string]any{
		"type": "struct",
		"fields": []any{
			map[string]any{"name": "weight", "type": "u16", "offset": 0, "scale": 0.1},
			map[string]any{"name": "flag", "type": "bit", "offset": 2, "bit": 0},
			map[string]any{"name": "counter", "type": "u8", "offset": 3},
			map[string]any{"name": "tag", "type": "string", "offset": 4, "length": 2},
		},
	}
	out, err := parseStructResult(parseCfg, raw)
	if err != nil {
		t.Fatalf("parse struct failed: %v", err)
	}
	weight, ok := out["weight"].(float64)
	if !ok || weight != 10.0 {
		t.Fatalf("unexpected weight: %#v", out["weight"])
	}
	flag, ok := out["flag"].(bool)
	if !ok || !flag {
		t.Fatalf("unexpected flag: %#v", out["flag"])
	}
	counter, ok := out["counter"].(int)
	if !ok || counter != 2 {
		t.Fatalf("unexpected counter: %#v", out["counter"])
	}
	tag, ok := out["tag"].(string)
	if !ok || tag != "AB" {
		t.Fatalf("unexpected tag: %#v", out["tag"])
	}
}
