package fakeweatherquery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/Tangerg/lynx/ai/model/chat"
	pkgJson "github.com/Tangerg/lynx/pkg/json"
)

var _ chat.CallableTool = (*FakeWeatherQuery)(nil)

// FakeWeatherQuery implements a fake weather query tool that generates simulated weather data.
//
// All weather information including temperature, precipitation, wind, air quality, and astronomical
// data is algorithmically generated based on climate zones and seasonal patterns. No real weather
// APIs or data sources are used.
//
// This tool is intended for:
//   - Testing and development of weather-dependent applications
//   - Demonstrations and prototyping
//   - Educational purposes
//   - Scenarios where real weather API access is not available or desired
//
// Warning: Do not use this tool for real-world weather forecasting or critical decision-making.
type FakeWeatherQuery struct {
	writer io.Writer
}

var inputSchema = pkgJson.StringDefSchemaOf(WeatherRequest{})

func (f *FakeWeatherQuery) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        "weather_query_api",
		Description: "Query weather information for a specific location and date. Supports current weather, forecasts, air quality, and astronomical data.",
		InputSchema: inputSchema,
	}
}

func (f *FakeWeatherQuery) Metadata() chat.ToolMetadata {
	return chat.ToolMetadata{}
}

func (f *FakeWeatherQuery) Call(ctx context.Context, arguments string) (string, error) {

	if f.writer == nil {
		f.writer = os.Stdout
	}

	f.log("raw_request", arguments)

	req := &WeatherRequest{}
	if err := json.Unmarshal([]byte(arguments), req); err != nil {
		return "", fmt.Errorf("failed to parse weather request: %w", err)
	}
	f.log("parsed_request", fmt.Sprintf("%#v", req))

	response, err := GenerateFakeWeatherResponse(req)
	if err != nil {
		return "", fmt.Errorf("failed to generate weather response: %w", err)
	}
	f.log("generated_response", fmt.Sprintf("%#v", response))

	responseData, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("failed to marshal weather response: %w", err)
	}

	responseStr := string(responseData)
	f.log("raw_response", responseStr)

	return responseStr, nil
}

func (f *FakeWeatherQuery) log(key, value string) {
	_, _ = fmt.Fprintf(f.writer, "[fake_weather_query] %s: %s\n", key, value)
}

func NewFakeWeatherQuery(writer io.Writer) *FakeWeatherQuery {
	if writer == nil {
		writer = os.Stdout
	}
	return &FakeWeatherQuery{
		writer: writer,
	}
}
