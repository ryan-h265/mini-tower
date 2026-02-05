package validate

import (
	"fmt"
	"math"
	"reflect"
)

var allowedTypes = map[string]bool{
	"object":  true,
	"array":   true,
	"string":  true,
	"number":  true,
	"integer": true,
	"boolean": true,
	"null":    true,
}

// ValidateJSONSchema validates a supported subset of JSON Schema.
func ValidateJSONSchema(schema map[string]any) error {
	if schema == nil {
		return nil
	}
	return validateSchemaNode(schema, "$")
}

// ValidateJSONInput validates input against a supported subset of JSON Schema.
func ValidateJSONInput(input any, schema map[string]any) error {
	if schema == nil {
		return nil
	}
	if input == nil && schemaWantsObject(schema) && !schemaAllowsType(schema, "null") {
		input = map[string]any{}
	}
	return validateValue(input, schema, "$")
}

func validateSchemaNode(schema map[string]any, path string) error {
	if t, ok := schema["type"]; ok {
		types, err := parseTypeList(t)
		if err != nil {
			return fmt.Errorf("%s.type: %w", path, err)
		}
		for _, typ := range types {
			if !allowedTypes[typ] {
				return fmt.Errorf("%s.type: unsupported type %q", path, typ)
			}
		}
	}

	if props, ok := schema["properties"]; ok {
		propMap, ok := props.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.properties must be an object", path)
		}
		for name, raw := range propMap {
			child, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("%s.properties.%s must be an object", path, name)
			}
			if err := validateSchemaNode(child, path+"."+name); err != nil {
				return err
			}
		}
	}

	if items, ok := schema["items"]; ok {
		child, ok := items.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.items must be an object", path)
		}
		if err := validateSchemaNode(child, path+".items"); err != nil {
			return err
		}
	}

	if req, ok := schema["required"]; ok {
		items, ok := req.([]any)
		if !ok {
			return fmt.Errorf("%s.required must be an array of strings", path)
		}
		for i, item := range items {
			if _, ok := item.(string); !ok {
				return fmt.Errorf("%s.required[%d] must be a string", path, i)
			}
		}
	}

	if ap, ok := schema["additionalProperties"]; ok {
		switch v := ap.(type) {
		case bool:
		case map[string]any:
			if err := validateSchemaNode(v, path+".additionalProperties"); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s.additionalProperties must be a boolean or object", path)
		}
	}

	if enum, ok := schema["enum"]; ok {
		if _, ok := enum.([]any); !ok {
			return fmt.Errorf("%s.enum must be an array", path)
		}
	}

	if v, ok := schema["minimum"]; ok {
		if _, ok := asNumber(v); !ok {
			return fmt.Errorf("%s.minimum must be a number", path)
		}
	}
	if v, ok := schema["maximum"]; ok {
		if _, ok := asNumber(v); !ok {
			return fmt.Errorf("%s.maximum must be a number", path)
		}
	}
	if v, ok := schema["minLength"]; ok {
		if _, ok := asInt(v); !ok {
			return fmt.Errorf("%s.minLength must be an integer", path)
		}
	}
	if v, ok := schema["maxLength"]; ok {
		if _, ok := asInt(v); !ok {
			return fmt.Errorf("%s.maxLength must be an integer", path)
		}
	}
	if v, ok := schema["minItems"]; ok {
		if _, ok := asInt(v); !ok {
			return fmt.Errorf("%s.minItems must be an integer", path)
		}
	}
	if v, ok := schema["maxItems"]; ok {
		if _, ok := asInt(v); !ok {
			return fmt.Errorf("%s.maxItems must be an integer", path)
		}
	}

	return nil
}

func validateValue(value any, schema map[string]any, path string) error {
	if enum, ok := schema["enum"].([]any); ok {
		matched := false
		for _, candidate := range enum {
			if reflect.DeepEqual(candidate, value) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s is not in enum", path)
		}
	}

	if t, ok := schema["type"]; ok {
		types, err := parseTypeList(t)
		if err != nil {
			return fmt.Errorf("%s.type: %w", path, err)
		}
		if !valueMatchesTypes(value, types) {
			return fmt.Errorf("%s has invalid type", path)
		}
	}

	switch v := value.(type) {
	case map[string]any:
		if err := validateObject(v, schema, path); err != nil {
			return err
		}
	case []any:
		if err := validateArray(v, schema, path); err != nil {
			return err
		}
	case string:
		if err := validateString(v, schema, path); err != nil {
			return err
		}
	case float64:
		if err := validateNumber(v, schema, path); err != nil {
			return err
		}
	case bool:
	case nil:
		if !schemaAllowsType(schema, "null") && schemaWantsObject(schema) {
			return fmt.Errorf("%s must be an object", path)
		}
	default:
		return fmt.Errorf("%s has unsupported type", path)
	}

	return nil
}

func validateObject(value map[string]any, schema map[string]any, path string) error {
	required := map[string]bool{}
	if req, ok := schema["required"].([]any); ok {
		for _, item := range req {
			if s, ok := item.(string); ok {
				required[s] = true
			}
		}
	}

	properties := map[string]map[string]any{}
	if props, ok := schema["properties"].(map[string]any); ok {
		for k, raw := range props {
			if child, ok := raw.(map[string]any); ok {
				properties[k] = child
			}
		}
	}

	additionalAllowed := true
	var additionalSchema map[string]any
	if ap, ok := schema["additionalProperties"]; ok {
		switch v := ap.(type) {
		case bool:
			additionalAllowed = v
		case map[string]any:
			additionalSchema = v
		}
	}

	for name := range required {
		if _, ok := value[name]; !ok {
			return fmt.Errorf("%s.%s is required", path, name)
		}
	}

	for name, val := range value {
		if propSchema, ok := properties[name]; ok {
			if err := validateValue(val, propSchema, path+"."+name); err != nil {
				return err
			}
			continue
		}
		if additionalSchema != nil {
			if err := validateValue(val, additionalSchema, path+"."+name); err != nil {
				return err
			}
			continue
		}
		if !additionalAllowed {
			return fmt.Errorf("%s.%s is not allowed", path, name)
		}
	}

	return nil
}

func validateArray(value []any, schema map[string]any, path string) error {
	if minItems, ok := schema["minItems"]; ok {
		if min, ok := asInt(minItems); ok && len(value) < min {
			return fmt.Errorf("%s has too few items", path)
		}
	}
	if maxItems, ok := schema["maxItems"]; ok {
		if max, ok := asInt(maxItems); ok && len(value) > max {
			return fmt.Errorf("%s has too many items", path)
		}
	}
	if items, ok := schema["items"].(map[string]any); ok {
		for i, item := range value {
			if err := validateValue(item, items, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateString(value string, schema map[string]any, path string) error {
	if minLength, ok := schema["minLength"]; ok {
		if min, ok := asInt(minLength); ok && len([]rune(value)) < min {
			return fmt.Errorf("%s is too short", path)
		}
	}
	if maxLength, ok := schema["maxLength"]; ok {
		if max, ok := asInt(maxLength); ok && len([]rune(value)) > max {
			return fmt.Errorf("%s is too long", path)
		}
	}
	return nil
}

func validateNumber(value float64, schema map[string]any, path string) error {
	if min, ok := schema["minimum"]; ok {
		if minVal, ok := asNumber(min); ok && value < minVal {
			return fmt.Errorf("%s is below minimum", path)
		}
	}
	if max, ok := schema["maximum"]; ok {
		if maxVal, ok := asNumber(max); ok && value > maxVal {
			return fmt.Errorf("%s is above maximum", path)
		}
	}
	return nil
}

func parseTypeList(v any) ([]string, error) {
	switch t := v.(type) {
	case string:
		return []string{t}, nil
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("type must be a string")
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("type must be a string or array of strings")
	}
}

func valueMatchesTypes(value any, types []string) bool {
	for _, typ := range types {
		if matchesType(value, typ) {
			return true
		}
	}
	return false
}

func matchesType(value any, typ string) bool {
	switch typ {
	case "null":
		return value == nil
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		_, ok := asNumber(value)
		return ok
	case "integer":
		_, ok := asInt(value)
		return ok
	default:
		return false
	}
}

func schemaAllowsType(schema map[string]any, typ string) bool {
	if t, ok := schema["type"]; ok {
		types, err := parseTypeList(t)
		if err != nil {
			return false
		}
		for _, entry := range types {
			if entry == typ {
				return true
			}
		}
		return false
	}
	return false
}

func schemaWantsObject(schema map[string]any) bool {
	if schemaAllowsType(schema, "object") {
		return true
	}
	if _, ok := schema["properties"]; ok {
		return true
	}
	if _, ok := schema["required"]; ok {
		return true
	}
	if _, ok := schema["additionalProperties"]; ok {
		return true
	}
	return false
}

func asNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func asInt(v any) (int, bool) {
	n, ok := asNumber(v)
	if !ok {
		return 0, false
	}
	if math.Trunc(n) != n {
		return 0, false
	}
	return int(n), true
}
