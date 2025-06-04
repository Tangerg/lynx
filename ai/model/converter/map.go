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
)

var _ StructuredOutputConverter[map[string]any] = (*MapOutputConverter)(nil)

// MapOutputConverter is a StructuredOutputConverter implementation that converts
// the LLM output into a map[string]any by parsing JSON format.
type MapOutputConverter struct {
}

// GetFormat returns the format instructions for the LLM to output JSON data
// that matches the golang map[string]interface{} format.
func (m *MapOutputConverter) GetFormat() string {
	return `
Your response should be in JSON format.
The data structure for the JSON should match golang map[string]interface{} format.
Do not include any explanations, only provide a RFC8259 compliant JSON response following this format without deviation.
Remove the` + " '```json' markdown surrounding the output including the trailing '```'."
}

// Convert converts the raw LLM output string into a map[string]any by parsing JSON.
// It automatically strips Markdown code blocks (```json and ```) if present.
func (m *MapOutputConverter) Convert(raw string) (map[string]any, error) {
	content := stripMarkdownCodeBlock(raw)
	rv := make(map[string]any)
	err := json.Unmarshal([]byte(content), &rv)
	if err != nil {
		return nil, errors.Join(err, fmt.Errorf("cannot convert content %s to map, raw: %s", content, raw))
	}
	return rv, nil
}
