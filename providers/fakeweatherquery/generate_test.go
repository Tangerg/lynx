package fakeweatherquery

import (
	"testing"
)

func TestGenerate(t *testing.T) {
	response, err := GenerateFakeWeatherResponse(&WeatherRequest{
		Location: "Beijing",
		Date:     "2024-01-15",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(response)
}
