package schema

import (
	"errors"
	"fmt"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

func Validate(compiled *jsonschema.Schema, value any) error {
	if compiled == nil {
		return fmt.Errorf("compiled schema is nil")
	}
	if err := compiled.Validate(value); err != nil {
		return errors.New(strings.TrimSpace(err.Error()))
	}
	return nil
}
