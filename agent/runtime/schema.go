package runtime

import (
	"errors"
	"fmt"

	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

func schemaFor(sample any) (string, error) {
	if sample == nil {
		return "", errors.New("input sample must not be nil")
	}
	schema, err := pkgjson.StringDefSchemaOf(sample)
	if err != nil {
		return "", fmt.Errorf("derive input schema: %w", err)
	}
	return schema, nil
}
