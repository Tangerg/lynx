/*
 * Copyright 2023-2024 the original author or authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package converter

import (
	"encoding/json"
	"errors"
	"fmt"

	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

var _ StructuredOutputConverter[any] = (*StructOutputConverter[any])(nil)

// StructOutputConverter is a generic StructuredOutputConverter implementation that converts
// the LLM output into a structured type T by parsing JSON format with JSON Schema validation.
type StructOutputConverter[T any] struct {
	format string
}

// genFormat generates the format instructions with JSON Schema for the LLM output.
// The schema is automatically derived from the generic type T.
func (s *StructOutputConverter[T]) genFormat() string {
	const template = `
Your response should be in JSON format.
Do not include any explanations, only provide a RFC8259 compliant JSON response following this format without deviation.
Do not include markdown code blocks in your response.
Remove the` + " '```json' markdown surrounding the output including the trailing '```'. " +
		`Here is the JSON Schema instance your output must adhere to: ` + "```%s```"
	var t T
	return fmt.Sprintf(template, pkgjson.StringDefSchemaOf(t))
}

// GetFormat returns the format instructions for the LLM to output JSON data
// that conforms to the JSON Schema of type T. The format is generated once and cached.
func (s *StructOutputConverter[T]) GetFormat() string {
	if s.format == "" {
		s.format = s.genFormat()
	}
	return s.format
}

// Convert converts the raw LLM output string into type T by parsing JSON.
// It automatically strips Markdown code blocks (```json and ```) if present.
func (s *StructOutputConverter[T]) Convert(raw string) (T, error) {
	content := stripMarkdownCodeBlock(raw)
	var t T
	err := json.Unmarshal([]byte(content), &t)
	if err != nil {
		return t, errors.Join(err, fmt.Errorf("cannot convert content %s to %T, raw: %s", content, t, raw))
	}
	return t, nil
}
