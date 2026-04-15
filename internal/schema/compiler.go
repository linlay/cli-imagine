package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

func Compile(resourceName string, schemaDef map[string]any) (*jsonschema.Schema, error) {
	if schemaDef == nil {
		return nil, fmt.Errorf("schema is required")
	}

	payload, err := json.Marshal(schemaDef)
	if err != nil {
		return nil, fmt.Errorf("marshal schema: %w", err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020

	resource := fmt.Sprintf("mem://%s.json", strings.TrimSpace(resourceName))
	if strings.TrimSpace(resourceName) == "" {
		resource = "mem://schema.json"
	}
	if err := compiler.AddResource(resource, bytes.NewReader(payload)); err != nil {
		return nil, fmt.Errorf("add schema resource: %w", err)
	}

	compiled, err := compiler.Compile(resource)
	if err != nil {
		return nil, fmt.Errorf("compile schema: %w", err)
	}
	return compiled, nil
}
