package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseArgumentsRequiresJSONObject(t *testing.T) {
	for _, input := range []string{"null", `[]`, `"text"`, `1`, `true`, `{]`, `{"name":"first","name":"second"}`} {
		t.Run(input, func(t *testing.T) {
			_, err := parseArguments(input)
			require.Error(t, err)
		})
	}

	const input = `{"integer":9007199254740993,"precise":1.0000000000000001}`
	got, err := parseArguments(input)
	require.NoError(t, err)
	assert.Equal(t, input, string(got))
}
