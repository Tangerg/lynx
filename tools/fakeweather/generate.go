package fakeweather

import (
	"fmt"
	"math"
	"math/rand/v2"
	"time"
)

// generate is the entry point: parse the request, derive every output
// field deterministically from (location, date), and return the
// Response. All randomness is seeded from the input — same input,
// same output across runs.
func generate(req *Request) (*Response, error) {
	target, err := parseTargetDate(req.Date)
	if err != nil {
		return nil, fmt.Errorf("fakeweather.generate: %w", err)
	}

	rng := newRng(req.Location, target)
	zone := identifyClimateZone(req.Location)
	coords, knownCity := coordinatesFor(req.Location, rng)
	seasonal := seasonalPatterns[zone]
	profile := climateProfiles[zone]

	month := monthForLookup(target, coords.Latitude, knownCity)

	// Daily mean for the (zone, month). For a date-only query this
	// IS the day's representative reading — we deliberately do NOT
	// apply diurnal variation, because that would put every
	// midnight-stamped query at the bottom of the daily curve.
	mean := profile.mean[month-1]

	// Elevation correction: ~0.6°C drop per 100 m.
	elevDrop := int(float64(coords.Elevation) * 0.006)
	mean -= elevDrop

	// Day-to-day jitter (±2°C) preserves variability without
	// breaking the seasonal floor.
	jitter := rng.IntN(5) - 2
	current := clamp(mean+jitter, profile.floor, profile.ceiling)

	// Min/Max describe the whole day's swing around the mean — not
	// random offsets from the "current" reading.
	minTemp := min(clamp(mean-profile.dailyAmplitude+rng.IntN(3)-1, profile.floor, profile.ceiling), current)
	maxTemp := max(clamp(mean+profile.dailyAmplitude+rng.IntN(3)-1, profile.floor, profile.ceiling), current)

	// Pick a condition compatible with the temperature + zone + month.
	candidates := candidateConditions(current, month, zone, seasonal)
	condition := candidates[rng.IntN(len(candidates))]

	wind := generateWind(condition, zone, coords, rng)
	humidity := generateHumidity(condition, zone, month, seasonal, rng)
	feelsLike := calculateFeelsLike(current, humidity, wind.Speed)
	pressure := generatePressure(coords.Elevation, condition, rng)
	visibility := generateVisibility(condition, humidity, rng)
	cloudCover := generateCloudCover(condition, rng)
	dewPoint := calculateDewPoint(current, humidity)

	var precipitation *Precipitation
	if precipitationFor(condition) {
		precipitation = generatePrecipitation(condition, current, month, seasonal, rng)
	}

	var airQuality *AirQuality
	if req.IncludeAirQuality {
		airQuality = generateAirQuality(req.Location, condition, zone, rng)
	}

	uvIndex := generateUVIndex(month, coords.Latitude, condition, cloudCover, rng)
	astronomy := generateAstronomy(target, coords, rng)

	var hourlyForecast []HourlyForecast
	if req.IncludeHourly {
		hourlyForecast = generateHourlyForecast(target, mean, condition, zone, profile, rng)
	}

	alerts := generateAlerts(condition, current, wind.Speed, zone, target, rng)
	description := buildDescription(condition, current, wind, humidity, precipitation)

	startOfDay := time.Date(target.Year(), target.Month(), target.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour)

	return &Response{
		Location:    req.Location,
		Coordinates: coords,
		Timestamp: TimeRange{
			Start: startOfDay.Unix(),
			End:   endOfDay.Unix(),
		},
		Temperature: Temperature{
			Value:     current,
			Unit:      "Celsius",
			FeelsLike: feelsLike,
			Min:       minTemp,
			Max:       maxTemp,
		},
		Condition:      condition,
		Description:    description,
		Humidity:       humidity,
		Pressure:       pressure,
		Visibility:     visibility,
		CloudCover:     cloudCover,
		DewPoint:       dewPoint,
		Wind:           wind,
		Precipitation:  precipitation,
		AirQuality:     airQuality,
		UVIndex:        uvIndex,
		Astronomy:      astronomy,
		HourlyForecast: hourlyForecast,
		Alerts:         alerts,
		Source:         "fakeweather (synthesized; not real weather data)",
		LastUpdated:    startOfDay.Unix(),
	}, nil
}

// parseTargetDate accepts an empty string (= today UTC) or
// "YYYY-MM-DD" and returns a time.Time pinned to 00:00 UTC of the
// target day.
func parseTargetDate(s string) (time.Time, error) {
	if s == "" {
		now := time.Now().UTC()
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("fakeweather.parseDate: invalid date %q (want YYYY-MM-DD): %w", s, err)
	}
	return t, nil
}

// newRng builds a deterministic PRNG seeded from location + date.
func newRng(location string, date time.Time) *rand.Rand {
	seed := uint64(date.Unix())
	for _, c := range location {
		seed = seed*31 + uint64(c)
	}
	return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
}

// monthForLookup decides which month to feed into the climate-profile
// table. For a known city in the southern hemisphere we flip months
// (so July reads as January temps). For unknown locations we use the
// month as-is — the previous algorithm pseudo-randomly assigned
// southern hemispheres to unknown cities, which produced "summer is
// freezing" reports.
func monthForLookup(date time.Time, latitude float64, knownCity bool) int {
	month := int(date.Month())
	if knownCity && latitude < 0 {
		month = ((month + 5) % 12) + 1 // 1..12 → flip 6 months
	}
	return month
}

// coordinatesFor returns the location's coordinates plus a flag for
// whether the lookup was a known-city hit. Unknown locations get
// derived (deterministic, northern-hemisphere) coords so the season
// math doesn't randomly flip; see [knownCities] for the gazetteer.
func coordinatesFor(location string, rng *rand.Rand) (Coordinates, bool) {
	if profile, ok := lookupCity(location); ok {
		return Coordinates{
			Latitude:  profile.Latitude,
			Longitude: profile.Longitude,
			Elevation: profile.Elevation,
		}, true
	}

	// Derive deterministic latitude in [10, 60] (mid-northern latitudes)
	// and longitude in [-180, 180] from the location string.
	var latSeed, lonSeed float64
	for _, c := range location {
		latSeed += float64(c)
		lonSeed += float64(c) * 1.5
	}
	lat := 10 + math.Mod(latSeed, 50)
	lon := math.Mod(lonSeed, 360) - 180
	elevation := rng.IntN(500)
	return Coordinates{
		Latitude:  math.Round(lat*10000) / 10000,
		Longitude: math.Round(lon*10000) / 10000,
		Elevation: elevation,
	}, false
}

// generateWind produces a Wind correlated with condition + zone +
// elevation. Speeds in km/h.
