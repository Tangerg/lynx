// Package fakeweather is a chat.CallableTool that returns synthesized
// weather data for a given (location, date). All values are
// deterministic functions of the input — there is no real weather API
// behind it.
//
// Use this for demos, prototypes, integration tests, or any flow that
// needs a "weather tool" that produces sane, reproducible JSON without
// network access. NEVER use the output for real decisions.
//
// Determinism: a request that supplies (location, date, includes)
// produces the same Response across runs. The seed is derived from
// the location string and target date.
package fakeweather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

// Request is the tool input. Date is optional; when empty the tool
// uses the current calendar date in UTC.
type Request struct {
	Location string `json:"location" jsonschema:"required" jsonschema_description:"Geographic location for weather query. Can be a city name (e.g., 'Beijing', 'New York', 'Tokyo'), city with country (e.g., 'Paris, France'), or specific address. Supports both English and local language names."`

	Date string `json:"date" jsonschema_description:"Target date for weather forecast in YYYY-MM-DD format (e.g., '2024-01-15'). If not provided or empty string, defaults to current UTC date. Must be a valid date string."`

	IncludeHourly bool `json:"include_hourly" jsonschema_description:"When true, returns a 24-element hour-by-hour forecast (temperature, condition, precipitation, humidity, wind speed). Default false."`

	IncludeAirQuality bool `json:"include_air_quality" jsonschema_description:"When true, returns AQI plus PM2.5/PM10/Ozone concentrations and a level description. Default false."`
}

// Response is the synthesized weather report.
type Response struct {
	Location       string           `json:"location"`
	Coordinates    Coordinates      `json:"coordinates"`
	Timestamp      TimeRange        `json:"timestamp"`
	Temperature    Temperature      `json:"temperature"`
	Condition      string           `json:"condition"`
	Description    string           `json:"description"`
	Humidity       int              `json:"humidity"`
	Pressure       int              `json:"pressure"`    // hPa
	Visibility     int              `json:"visibility"`  // km
	CloudCover     int              `json:"cloud_cover"` // 0-100
	DewPoint       int              `json:"dew_point"`
	Wind           Wind             `json:"wind"`
	Precipitation  *Precipitation   `json:"precipitation,omitempty"`
	AirQuality     *AirQuality      `json:"air_quality,omitempty"`
	UVIndex        UVIndex          `json:"uv_index"`
	Astronomy      Astronomy        `json:"astronomy"`
	HourlyForecast []HourlyForecast `json:"hourly_forecast,omitempty"`
	Alerts         []Alert          `json:"alerts,omitempty"`
	Source         string           `json:"source"`
	LastUpdated    int64            `json:"last_updated"` // Unix seconds, equal to start of target date (deterministic)
}

// Coordinates is the location's geographic anchor. Elevation in metres.
type Coordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Elevation int     `json:"elevation"`
}

// Temperature is the day's representative temperature plus its swing.
// Value is the daily mean (not midnight, not noon) so a date-only
// query gets the day's "typical" reading.
type Temperature struct {
	Value     int    `json:"value"`
	Unit      string `json:"unit"` // always "Celsius"
	FeelsLike int    `json:"feels_like"`
	Min       int    `json:"min"`
	Max       int    `json:"max"`
}

// Wind in km/h.
type Wind struct {
	Speed     float64 `json:"speed"`
	Unit      string  `json:"unit"` // always "km/h"
	Direction string  `json:"direction"`
	Degree    int     `json:"degree"`
	Gust      float64 `json:"gust"`
}

// Precipitation describes liquid/solid water expected for the day.
type Precipitation struct {
	Type        string  `json:"type"`        // rain, snow, sleet
	Probability int     `json:"probability"` // 0-100
	Amount      float64 `json:"amount"`      // mm
	Intensity   string  `json:"intensity"`   // light, moderate, heavy
}

// AirQuality is the AQI + breakdown.
type AirQuality struct {
	AQI         int    `json:"aqi"`
	Level       string `json:"level"`
	PM25        int    `json:"pm2_5"`
	PM10        int    `json:"pm10"`
	Ozone       int    `json:"ozone"`
	Description string `json:"description"`
}

// UVIndex per WHO levels (0-11+).
type UVIndex struct {
	Value       int    `json:"value"`
	Level       string `json:"level"`
	Description string `json:"description"`
}

// Astronomy holds sun + moon data, all times in HH:MM (location's local
// time inferred from longitude — approximate).
type Astronomy struct {
	Sunrise          string `json:"sunrise"`
	Sunset           string `json:"sunset"`
	Moonrise         string `json:"moonrise"`
	Moonset          string `json:"moonset"`
	MoonPhase        string `json:"moon_phase"`
	MoonIllumination int    `json:"moon_illumination"` // 0-100
}

// TimeRange is a [start, end) Unix-second window covering the target date.
type TimeRange struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// HourlyForecast is one hour of the 24-hour breakdown.
type HourlyForecast struct {
	Time          int64   `json:"time"`
	Temperature   int     `json:"temperature"`
	Condition     string  `json:"condition"`
	Precipitation float64 `json:"precipitation"`
	Humidity      int     `json:"humidity"`
	WindSpeed     float64 `json:"wind_speed"`
}

// Alert is a synthesized weather alert (heat, cold, wind, storm,
// blizzard, typhoon).
type Alert struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"` // minor, moderate, severe, extreme
	Title       string `json:"title"`
	Description string `json:"description"`
	StartTime   int64  `json:"start_time"`
	EndTime     int64  `json:"end_time"`
}

var _ chat.CallableTool = (*Tool)(nil)

// Tool is a chat.CallableTool that synthesizes weather reports.
// Construct with [New].
type Tool struct {
	writer io.Writer
}

// New returns a Tool that writes its trace lines to writer. Pass nil
// to suppress trace output (writer = io.Discard).
func New(writer io.Writer) *Tool {
	if writer == nil {
		writer = io.Discard
	}
	return &Tool{writer: writer}
}

var inputSchema, _ = pkgjson.StringDefSchemaOf(Request{})

func (t *Tool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        "weather_query",
		Description: "Query synthesized weather information for a location and date. Supports current weather, forecasts, air quality, and astronomical data. Output is deterministic, not real.",
		InputSchema: inputSchema,
	}
}

func (t *Tool) Metadata() chat.ToolMetadata { return chat.ToolMetadata{} }

func (t *Tool) Call(_ context.Context, arguments string) (string, error) {
	t.log("raw_request", arguments)

	req := &Request{}
	if err := json.Unmarshal([]byte(arguments), req); err != nil {
		return "", fmt.Errorf("fakeweather.Tool.Call: parse arguments: %w", err)
	}
	t.log("parsed_request", fmt.Sprintf("%#v", req))

	resp, err := generate(req)
	if err != nil {
		return "", fmt.Errorf("fakeweather.Tool.Call: %w", err)
	}
	t.log("generated_response", fmt.Sprintf("%#v", resp))

	body, err := json.Marshal(resp)
	if err != nil {
		return "", fmt.Errorf("fakeweather.Tool.Call: marshal response: %w", err)
	}
	out := string(body)
	t.log("raw_response", out)
	return out, nil
}

func (t *Tool) log(key, value string) {
	_, _ = fmt.Fprintf(t.writer, "[fakeweather] %s: %s\n", key, value)
}
