package jsonschema

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
)

type Draft struct{}

var Draft2020 = &Draft{}

type Compiler struct {
	Draft     *Draft
	resources map[string]map[string]any
}

func NewCompiler() *Compiler {
	return &Compiler{resources: map[string]map[string]any{}}
}

func (c *Compiler) AddResource(url string, r io.Reader) error {
	if c.resources == nil {
		c.resources = map[string]map[string]any{}
	}
	payload, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	var node any
	if err := json.Unmarshal(payload, &node); err != nil {
		return err
	}
	root, ok := node.(map[string]any)
	if !ok {
		return fmt.Errorf("schema root must be object")
	}
	if err := validateSchemaNode(root, "$"); err != nil {
		return err
	}
	c.resources[url] = root
	return nil
}

func (c *Compiler) Compile(url string) (*Schema, error) {
	if c.resources == nil {
		return nil, fmt.Errorf("no schema resources")
	}
	root, ok := c.resources[url]
	if !ok {
		return nil, fmt.Errorf("schema resource not found: %s", url)
	}
	return &Schema{root: root}, nil
}

type Schema struct {
	root map[string]any
}

func (s *Schema) Validate(value any) error {
	if s == nil || s.root == nil {
		return fmt.Errorf("nil schema")
	}
	normalized, err := normalize(value)
	if err != nil {
		return err
	}
	return validateValue(s.root, normalized, "$")
}

func normalize(value any) (any, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func validateSchemaNode(node map[string]any, path string) error {
	if rawType, ok := node["type"]; ok {
		if err := validateTypeKeyword(rawType, path+".type"); err != nil {
			return err
		}
	}
	if rawRequired, ok := node["required"]; ok {
		required, ok := rawRequired.([]any)
		if !ok {
			return fmt.Errorf("%s.required must be array", path)
		}
		for _, item := range required {
			if _, ok := item.(string); !ok {
				return fmt.Errorf("%s.required item must be string", path)
			}
		}
	}
	if rawProps, ok := node["properties"]; ok {
		props, ok := rawProps.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.properties must be object", path)
		}
		for key, rawChild := range props {
			child, ok := rawChild.(map[string]any)
			if !ok {
				return fmt.Errorf("%s.properties.%s must be object", path, key)
			}
			if err := validateSchemaNode(child, path+".properties."+key); err != nil {
				return err
			}
		}
	}
	if rawItems, ok := node["items"]; ok {
		items, ok := rawItems.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.items must be object", path)
		}
		if err := validateSchemaNode(items, path+".items"); err != nil {
			return err
		}
	}
	if rawAdditional, ok := node["additionalProperties"]; ok {
		switch typed := rawAdditional.(type) {
		case bool:
		case map[string]any:
			if err := validateSchemaNode(typed, path+".additionalProperties"); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s.additionalProperties must be boolean or object", path)
		}
	}
	if rawMinLength, ok := node["minLength"]; ok {
		if number(rawMinLength) < 0 {
			return fmt.Errorf("%s.minLength must be >= 0", path)
		}
	}
	if rawMin, ok := node["minimum"]; ok {
		if !isNumber(rawMin) {
			return fmt.Errorf("%s.minimum must be number", path)
		}
	}
	if rawMax, ok := node["maximum"]; ok {
		if !isNumber(rawMax) {
			return fmt.Errorf("%s.maximum must be number", path)
		}
	}
	return nil
}

func validateTypeKeyword(raw any, path string) error {
	switch typed := raw.(type) {
	case string:
		if !isAllowedType(typed) {
			return fmt.Errorf("%s has unsupported type %q", path, typed)
		}
		return nil
	case []any:
		if len(typed) == 0 {
			return fmt.Errorf("%s type array cannot be empty", path)
		}
		for _, item := range typed {
			value, ok := item.(string)
			if !ok || !isAllowedType(value) {
				return fmt.Errorf("%s has unsupported type entry", path)
			}
		}
		return nil
	default:
		return fmt.Errorf("%s must be string or array", path)
	}
}

func isAllowedType(value string) bool {
	switch value {
	case "object", "array", "string", "number", "integer", "boolean", "null":
		return true
	default:
		return false
	}
}

func validateValue(schema map[string]any, value any, path string) error {
	if rawConst, ok := schema["const"]; ok {
		if !reflect.DeepEqual(rawConst, value) {
			return fmt.Errorf("%s must equal const", path)
		}
	}
	if rawEnum, ok := schema["enum"]; ok {
		list, ok := rawEnum.([]any)
		if !ok {
			return fmt.Errorf("%s enum must be array", path)
		}
		matched := false
		for _, item := range list {
			if reflect.DeepEqual(item, value) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s must be one of enum", path)
		}
	}

	if rawType, ok := schema["type"]; ok {
		if !matchType(rawType, value) {
			return fmt.Errorf("%s type mismatch", path)
		}
	}

	switch typed := value.(type) {
	case map[string]any:
		if err := validateObject(schema, typed, path); err != nil {
			return err
		}
	case []any:
		if err := validateArray(schema, typed, path); err != nil {
			return err
		}
	case string:
		if rawMinLength, ok := schema["minLength"]; ok {
			if len([]rune(typed)) < int(number(rawMinLength)) {
				return fmt.Errorf("%s length must be >= %d", path, int(number(rawMinLength)))
			}
		}
	}

	if rawMin, ok := schema["minimum"]; ok && isNumber(value) {
		if number(value) < number(rawMin) {
			return fmt.Errorf("%s must be >= %v", path, rawMin)
		}
	}
	if rawMax, ok := schema["maximum"]; ok && isNumber(value) {
		if number(value) > number(rawMax) {
			return fmt.Errorf("%s must be <= %v", path, rawMax)
		}
	}
	return nil
}

func validateObject(schema map[string]any, object map[string]any, path string) error {
	required := map[string]struct{}{}
	if rawRequired, ok := schema["required"]; ok {
		for _, item := range rawRequired.([]any) {
			required[item.(string)] = struct{}{}
		}
	}
	for key := range required {
		if _, ok := object[key]; !ok {
			return fmt.Errorf("%s.%s is required", path, key)
		}
	}

	properties := map[string]map[string]any{}
	if rawProps, ok := schema["properties"]; ok {
		for key, rawChild := range rawProps.(map[string]any) {
			properties[key] = rawChild.(map[string]any)
		}
	}

	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := object[key]
		if child, ok := properties[key]; ok {
			if err := validateValue(child, value, path+"."+key); err != nil {
				return err
			}
			continue
		}
		if rawAdditional, ok := schema["additionalProperties"]; ok {
			switch typed := rawAdditional.(type) {
			case bool:
				if !typed {
					return fmt.Errorf("%s.%s is not allowed", path, key)
				}
			case map[string]any:
				if err := validateValue(typed, value, path+"."+key); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateArray(schema map[string]any, items []any, path string) error {
	rawItems, ok := schema["items"]
	if !ok {
		return nil
	}
	child, ok := rawItems.(map[string]any)
	if !ok {
		return fmt.Errorf("%s.items must be object", path)
	}
	for idx, item := range items {
		if err := validateValue(child, item, fmt.Sprintf("%s[%d]", path, idx)); err != nil {
			return err
		}
	}
	return nil
}

func matchType(rawType any, value any) bool {
	switch typed := rawType.(type) {
	case string:
		return matchSingleType(typed, value)
	case []any:
		for _, item := range typed {
			name, ok := item.(string)
			if ok && matchSingleType(name, value) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func matchSingleType(expected string, value any) bool {
	switch expected {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		return isNumber(value)
	case "integer":
		if !isNumber(value) {
			return false
		}
		v := number(value)
		return float64(int64(v)) == v
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "null":
		return value == nil
	default:
		return false
	}
}

func isNumber(value any) bool {
	switch value.(type) {
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	default:
		return false
	}
}

func number(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int8:
		return float64(typed)
	case int16:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	case uint:
		return float64(typed)
	case uint8:
		return float64(typed)
	case uint16:
		return float64(typed)
	case uint32:
		return float64(typed)
	case uint64:
		return float64(typed)
	default:
		return 0
	}
}

func (s *Schema) String() string {
	if s == nil || s.root == nil {
		return "{}"
	}
	payload, _ := json.Marshal(s.root)
	return strings.TrimSpace(string(payload))
}
