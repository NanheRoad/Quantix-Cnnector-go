package service

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"quantix-connector-go/internal/driver"

	"github.com/expr-lang/expr"
)

var placeholderRe = regexp.MustCompile(`\$\{([^}]+)\}`)

type ProtocolExecutor struct{}

func NewProtocolExecutor() *ProtocolExecutor { return &ProtocolExecutor{} }

func (e *ProtocolExecutor) RunSetupSteps(ctx context.Context, template map[string]any, drv driver.Driver, variables map[string]any) (map[string]any, error) {
	contextMap := map[string]any{"steps": map[string]any{}}
	for k, v := range variables {
		contextMap[k] = v
	}
	steps, _ := template["setup_steps"].([]any)
	for _, raw := range steps {
		step, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		res, err := e.executeStep(ctx, drv, step, contextMap, nil, false)
		if err != nil {
			return nil, err
		}
		stepsMap := contextMap["steps"].(map[string]any)
		stepsMap[fmt.Sprintf("%v", step["id"])] = map[string]any{"result": res}
	}
	return contextMap["steps"].(map[string]any), nil
}

func (e *ProtocolExecutor) RunPollSteps(ctx context.Context, template map[string]any, drv driver.Driver, variables map[string]any, previous map[string]any) (map[string]any, error) {
	stepsResults := copyMapExec(previous)
	contextMap := map[string]any{"steps": stepsResults}
	for k, v := range variables {
		contextMap[k] = v
	}
	steps, _ := template["steps"].([]any)
	for _, raw := range steps {
		step, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if fmt.Sprintf("%v", execValueOr(step["trigger"], "poll")) != "poll" {
			continue
		}
		res, err := e.executeStep(ctx, drv, step, contextMap, nil, false)
		if err != nil {
			return nil, err
		}
		stepsResults[fmt.Sprintf("%v", step["id"])] = map[string]any{"result": res}
	}
	return stepsResults, nil
}

func (e *ProtocolExecutor) RunManualStep(ctx context.Context, template map[string]any, drv driver.Driver, stepID string, variables map[string]any, paramsOverride map[string]any, previous map[string]any) (map[string]any, error) {
	contextMap := map[string]any{"steps": copyMapExec(previous)}
	for k, v := range variables {
		contextMap[k] = v
	}
	steps, _ := template["steps"].([]any)
	var target map[string]any
	for _, raw := range steps {
		step, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if fmt.Sprintf("%v", step["id"]) == stepID {
			target = step
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("Step not found: %s", stepID)
	}
	if fmt.Sprintf("%v", execValueOr(target["trigger"], "poll")) != "manual" {
		return nil, fmt.Errorf("Step is not manual trigger: %s", stepID)
	}
	result, err := e.executeStep(ctx, drv, target, contextMap, paramsOverride, false)
	if err != nil {
		return nil, err
	}
	stepsMap := contextMap["steps"].(map[string]any)
	stepsMap[fmt.Sprintf("%v", target["id"])] = map[string]any{"result": result}
	return map[string]any{
		"step_id": target["id"],
		"result":  result,
		"output":  e.RenderOutput(template, contextMap),
	}, nil
}

func (e *ProtocolExecutor) RunMessageHandler(ctx context.Context, template map[string]any, drv driver.Driver, payload []byte, variables map[string]any, previous map[string]any) (map[string]any, map[string]any, error) {
	handler, _ := template["message_handler"].(map[string]any)
	if handler == nil {
		return nil, nil, fmt.Errorf("Template has no message_handler")
	}
	contextMap := map[string]any{
		"payload": string(payload),
		"steps":   copyMapExec(previous),
	}
	for k, v := range variables {
		contextMap[k] = v
	}
	result, err := e.executeStep(ctx, drv, handler, contextMap, nil, true)
	if err != nil {
		return nil, nil, err
	}
	contextMap["message_handler"] = map[string]any{"result": result}
	stepsMap := contextMap["steps"].(map[string]any)
	return stepsMap, e.RenderOutput(template, contextMap), nil
}

func (e *ProtocolExecutor) RenderOutput(template map[string]any, contextMap map[string]any) map[string]any {
	output, ok := template["output"].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	v := e.resolveValue(output, contextMap)
	m, ok := v.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return m
}

func (e *ProtocolExecutor) ExecuteOneStep(ctx context.Context, drv driver.Driver, step map[string]any, contextMap map[string]any, paramsOverride map[string]any, skipDriver bool) (any, error) {
	return e.executeStep(ctx, drv, step, contextMap, paramsOverride, skipDriver)
}

func (e *ProtocolExecutor) executeStep(ctx context.Context, drv driver.Driver, step map[string]any, contextMap map[string]any, paramsOverride map[string]any, skipDriver bool) (any, error) {
	action := fmt.Sprintf("%v", step["action"])
	params, _ := e.resolveValue(execValueOr(step["params"], map[string]any{}), contextMap).(map[string]any)
	if params == nil {
		params = map[string]any{}
	}
	for k, v := range paramsOverride {
		params[k] = v
	}
	var (
		raw any
		err error
	)
	switch {
	case action == "delay":
		delayMS := int(anyToFloat(execValueOr(params["milliseconds"], 0)))
		if delayMS > 0 {
			timer := time.NewTimer(time.Duration(delayMS) * time.Millisecond)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		raw = map[string]any{"delayed_ms": delayMS}
	case strings.HasPrefix(action, "transform."):
		raw, err = runTransform(action, params)
	case skipDriver:
		raw = map[string]any{"payload": contextMap["payload"]}
	default:
		raw, err = drv.ExecuteAction(ctx, action, params)
	}
	if err != nil {
		return nil, err
	}
	parseCfg, hasParse := step["parse"].(map[string]any)
	if !hasParse {
		return raw, nil
	}
	return e.parseResult(parseCfg, raw, contextMap)
}

func runTransform(action string, params map[string]any) (any, error) {
	input := transformInputString(execValueOr(params["input"], ""))
	switch action {
	case "transform.base64_decode":
		return base64.StdEncoding.DecodeString(input)
	case "transform.hex_decode":
		src := strings.ReplaceAll(input, " ", "")
		return hexDecode(src)
	case "transform.regex_extract":
		re, err := regexp.Compile(fmt.Sprintf("%v", params["pattern"]))
		if err != nil {
			return nil, err
		}
		match := re.FindStringSubmatch(input)
		if len(match) == 0 {
			return nil, nil
		}
		idx := int(anyToFloat(execValueOr(params["group"], 1)))
		if idx < 0 || idx >= len(match) {
			return nil, nil
		}
		return match[idx], nil
	case "transform.substring":
		start := int(anyToFloat(execValueOr(params["start"], 0)))
		end := int(anyToFloat(execValueOr(params["end"], len(input))))
		if start < 0 {
			start = 0
		}
		if end > len(input) {
			end = len(input)
		}
		if start > end {
			start = end
		}
		return input[start:end], nil
	default:
		return nil, fmt.Errorf("Unsupported transform action: %s", action)
	}
}

func transformInputString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	case map[string]any:
		// Common pattern: pass previous step raw result object into transform.
		if p, ok := t["payload"]; ok {
			switch b := p.(type) {
			case []byte:
				return string(b)
			case string:
				return b
			default:
				return fmt.Sprintf("%v", b)
			}
		}
		return fmt.Sprintf("%v", t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func (e *ProtocolExecutor) parseResult(parseCfg map[string]any, raw any, contextMap map[string]any) (any, error) {
	parseType := fmt.Sprintf("%v", parseCfg["type"])
	switch parseType {
	case "expression":
		return evalExpression(fmt.Sprintf("%v", parseCfg["expression"]), raw, contextMap)
	case "regex":
		text := extractPayload(raw)
		re, err := regexp.Compile(fmt.Sprintf("%v", parseCfg["pattern"]))
		if err != nil {
			return nil, err
		}
		match := re.FindStringSubmatch(text)
		if len(match) == 0 {
			return nil, nil
		}
		group := int(anyToFloat(execValueOr(parseCfg["group"], 1)))
		if group < 0 || group >= len(match) {
			return nil, nil
		}
		return match[group], nil
	case "substring":
		text := extractPayload(raw)
		start := int(anyToFloat(execValueOr(parseCfg["start"], 0)))
		end := int(anyToFloat(execValueOr(parseCfg["end"], len(text))))
		if start < 0 {
			start = 0
		}
		if end > len(text) {
			end = len(text)
		}
		if start > end {
			start = end
		}
		return text[start:end], nil
	case "struct":
		return parseStructResult(parseCfg, raw)
	default:
		return nil, fmt.Errorf("Unsupported parse type: %s", parseType)
	}
}

func parseStructResult(parseCfg map[string]any, raw any) (map[string]any, error) {
	payload := extractPayloadRaw(raw, fmt.Sprintf("%v", execValueOr(parseCfg["payload_encoding"], "")))
	result := map[string]any{
		"_payload_hex":  fmt.Sprintf("% x", payload),
		"_payload_size": len(payload),
	}
	fields, _ := parseCfg["fields"].([]any)
	if len(fields) == 0 {
		result["payload_text"] = string(payload)
		return result, nil
	}
	cursor := 0
	for _, fr := range fields {
		field, _ := fr.(map[string]any)
		if field == nil {
			continue
		}
		name := strings.TrimSpace(fmt.Sprintf("%v", field["name"]))
		if name == "" {
			continue
		}
		offset := int(anyToFloat(execValueOr(field["offset"], cursor)))
		if offset < 0 {
			offset = 0
		}
		typ := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", execValueOr(field["type"], "u16"))))
		length := int(anyToFloat(execValueOr(field["length"], 0)))
		endian := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", execValueOr(field["endian"], "big"))))
		value, next, err := parseStructField(payload, offset, length, typ, endian, field)
		if err != nil {
			return nil, fmt.Errorf("field %s parse failed: %w", name, err)
		}
		result[name] = applyStructScale(value, field)
		cursor = next
	}
	return result, nil
}

func extractPayloadRaw(raw any, encoding string) []byte {
	switch v := raw.(type) {
	case []byte:
		return append([]byte{}, v...)
	case map[string]any:
		p, ok := v["payload"]
		if !ok || p == nil {
			return []byte{}
		}
		switch pv := p.(type) {
		case []byte:
			return append([]byte{}, pv...)
		case string:
			if strings.EqualFold(strings.TrimSpace(encoding), "hex") {
				clean := strings.ReplaceAll(strings.TrimSpace(pv), " ", "")
				decoded, err := hex.DecodeString(clean)
				if err == nil {
					return decoded
				}
			}
			return []byte(pv)
		default:
			return []byte(fmt.Sprintf("%v", pv))
		}
	case string:
		if strings.EqualFold(strings.TrimSpace(encoding), "hex") {
			clean := strings.ReplaceAll(strings.TrimSpace(v), " ", "")
			decoded, err := hex.DecodeString(clean)
			if err == nil {
				return decoded
			}
		}
		return []byte(v)
	default:
		return []byte(fmt.Sprintf("%v", v))
	}
}

func parseStructField(payload []byte, offset, length int, typ, endian string, field map[string]any) (any, int, error) {
	if offset > len(payload) {
		return nil, offset, fmt.Errorf("offset out of range: %d > %d", offset, len(payload))
	}
	var order binary.ByteOrder = binary.BigEndian
	if endian == "little" || endian == "le" {
		order = binary.LittleEndian
	}
	switch typ {
	case "u8":
		b, err := readStructBytes(payload, offset, 1)
		if err != nil {
			return nil, offset, err
		}
		return int(b[0]), offset + 1, nil
	case "i8":
		b, err := readStructBytes(payload, offset, 1)
		if err != nil {
			return nil, offset, err
		}
		return int(int8(b[0])), offset + 1, nil
	case "u16":
		b, err := readStructBytes(payload, offset, 2)
		if err != nil {
			return nil, offset, err
		}
		return int(order.Uint16(b)), offset + 2, nil
	case "i16":
		b, err := readStructBytes(payload, offset, 2)
		if err != nil {
			return nil, offset, err
		}
		return int(int16(order.Uint16(b))), offset + 2, nil
	case "u32":
		b, err := readStructBytes(payload, offset, 4)
		if err != nil {
			return nil, offset, err
		}
		return int64(order.Uint32(b)), offset + 4, nil
	case "i32":
		b, err := readStructBytes(payload, offset, 4)
		if err != nil {
			return nil, offset, err
		}
		return int64(int32(order.Uint32(b))), offset + 4, nil
	case "f32", "float32":
		b, err := readStructBytes(payload, offset, 4)
		if err != nil {
			return nil, offset, err
		}
		return float64(math.Float32frombits(order.Uint32(b))), offset + 4, nil
	case "f64", "float64":
		b, err := readStructBytes(payload, offset, 8)
		if err != nil {
			return nil, offset, err
		}
		return math.Float64frombits(order.Uint64(b)), offset + 8, nil
	case "bit":
		b, err := readStructBytes(payload, offset, 1)
		if err != nil {
			return nil, offset, err
		}
		bitIndex := int(anyToFloat(execValueOr(field["bit"], 0)))
		if bitIndex < 0 || bitIndex > 7 {
			return nil, offset, fmt.Errorf("bit index out of range: %d", bitIndex)
		}
		mask := byte(1 << uint(bitIndex))
		return b[0]&mask != 0, offset + 1, nil
	case "bool":
		b, err := readStructBytes(payload, offset, 1)
		if err != nil {
			return nil, offset, err
		}
		return b[0] != 0, offset + 1, nil
	case "bytes":
		n := length
		if n <= 0 {
			n = len(payload) - offset
		}
		b, err := readStructBytes(payload, offset, n)
		if err != nil {
			return nil, offset, err
		}
		return append([]byte{}, b...), offset + n, nil
	case "string":
		n := length
		if n <= 0 {
			n = len(payload) - offset
		}
		b, err := readStructBytes(payload, offset, n)
		if err != nil {
			return nil, offset, err
		}
		return strings.TrimRight(string(b), "\x00"), offset + n, nil
	default:
		return nil, offset, fmt.Errorf("unsupported struct field type: %s", typ)
	}
}

func readStructBytes(payload []byte, offset, length int) ([]byte, error) {
	if length < 0 {
		return nil, fmt.Errorf("invalid length: %d", length)
	}
	end := offset + length
	if end > len(payload) {
		return nil, fmt.Errorf("payload too short: need=%d have=%d", end, len(payload))
	}
	return payload[offset:end], nil
}

func applyStructScale(value any, field map[string]any) any {
	scaleRaw, ok := field["scale"]
	if !ok || scaleRaw == nil {
		return value
	}
	scale := anyToFloat(scaleRaw)
	switch v := value.(type) {
	case int:
		return float64(v) * scale
	case int64:
		return float64(v) * scale
	case float64:
		return v * scale
	default:
		return value
	}
}

func evalExpression(expression string, raw any, contextMap map[string]any) (any, error) {
	env := map[string]any{
		"steps": contextMap["steps"],
		"int":   func(v any) int { return int(anyToFloat(v)) },
		// Return nil for non-numeric values to avoid masking parse failures as 0.
		"float": func(v any) any {
			if n, ok := anyToFloatStrict(v); ok {
				return n
			}
			return nil
		},
		"str": func(v any) string { return fmt.Sprintf("%v", v) },
	}
	for k, v := range contextMap {
		env[k] = v
	}
	if m, ok := raw.(map[string]any); ok {
		if regs, ok := m["registers"]; ok {
			env["registers"] = regs
		}
		if coils, ok := m["coils"]; ok {
			env["coils"] = coils
		}
	}
	env["payload"] = extractPayload(raw)
	program, err := expr.Compile(expression, expr.Env(env))
	if err != nil {
		return nil, err
	}
	out, err := expr.Run(program, env)
	if err != nil {
		text := strings.ToLower(err.Error())
		if strings.Contains(text, "float") || strings.Contains(text, "int") {
			return nil, nil
		}
		return nil, err
	}
	return out, nil
}

func extractPayload(raw any) string {
	switch v := raw.(type) {
	case nil:
		return ""
	case []byte:
		return string(v)
	case map[string]any:
		p, ok := v["payload"]
		if !ok || p == nil {
			return ""
		}
		switch b := p.(type) {
		case []byte:
			return string(b)
		default:
			return fmt.Sprintf("%v", b)
		}
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (e *ProtocolExecutor) resolveValue(value any, contextMap map[string]any) any {
	switch t := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, v := range t {
			out[k] = e.resolveValue(v, contextMap)
		}
		return out
	case []any:
		out := make([]any, 0, len(t))
		for _, v := range t {
			out = append(out, e.resolveValue(v, contextMap))
		}
		return out
	case string:
		matches := placeholderRe.FindAllStringSubmatch(t, -1)
		if len(matches) == 0 {
			return t
		}
		if len(matches) == 1 && strings.TrimSpace(t) == "${"+matches[0][1]+"}" {
			return getFromContext(matches[0][1], contextMap)
		}
		out := t
		for _, m := range matches {
			path := m[1]
			v := getFromContext(path, contextMap)
			replace := ""
			if v != nil {
				replace = fmt.Sprintf("%v", v)
			}
			out = strings.ReplaceAll(out, "${"+path+"}", replace)
		}
		return out
	default:
		return value
	}
}

func getFromContext(path string, contextMap map[string]any) any {
	parts := strings.Split(path, ".")
	var current any = contextMap
	for _, part := range parts {
		switch node := current.(type) {
		case map[string]any:
			current = node[part]
		default:
			return nil
		}
	}
	return current
}

func anyToFloat(v any) float64 {
	n, _ := anyToFloatStrict(v)
	return n
}

func anyToFloatStrict(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case uint:
		return float64(t), true
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func execValueOr(v, fallback any) any {
	if v == nil {
		return fallback
	}
	return v
}

func hexDecode(clean string) ([]byte, error) {
	if clean == "" {
		return []byte{}, nil
	}
	clean = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(clean)), "0x")
	if len(clean)%2 != 0 {
		clean = "0" + clean
	}
	return hex.DecodeString(clean)
}

func copyMapExec(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
