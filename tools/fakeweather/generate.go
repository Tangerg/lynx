package fakeweather

import (
	"fmt"
	"math"
	"math/rand/v2"
	"time"
)

type reportGenerator struct {
	request  Request
	target   time.Time
	rng      *rand.Rand
	zone     climateZone
	coords   Coordinates
	seasonal seasonalPattern
	profile  climateProfile
	month    int
}

// generate is the entry point: parse the request, derive every output
// field deterministically from (location, date), and return the
// Response. All randomness is seeded from the input — same input,
// same output across runs.
func generate(req *Request) (*Response, error) {
	generator, err := newReportGenerator(req)
	if err != nil {
		return nil, fmt.Errorf("fakeweather.generate: %w", err)
	}
	return generator.report(), nil
}

func newReportGenerator(req *Request) (*reportGenerator, error) {
	target, err := parseTargetDate(req.Date)
	if err != nil {
		return nil, err
	}

	rng := newRng(req.Location, target)
	zone := identifyClimateZone(req.Location)
	coords, knownCity := coordinatesFor(req.Location, rng)
	return &reportGenerator{
		request:  *req,
		target:   target,
		rng:      rng,
		zone:     zone,
		coords:   coords,
		seasonal: seasonalPatterns[zone],
		profile:  climateProfiles[zone],
		month:    monthForLookup(target, coords.Latitude, knownCity),
	}, nil
}

func (g *reportGenerator) report() *Response {
	// Daily mean for the (zone, month). For a date-only query this
	// IS the day's representative reading — diurnal variation is deliberately
	// NOT applied, because that would put every
	// midnight-stamped query at the bottom of the daily curve.
	mean := g.profile.mean[g.month-1]

	// Elevation correction: ~0.6°C drop per 100 m.
	elevDrop := int(float64(g.coords.Elevation) * 0.006)
	mean -= elevDrop

	// Day-to-day jitter (±2°C) preserves variability without
	// breaking the seasonal floor.
	jitter := g.rng.IntN(5) - 2
	current := clamp(mean+jitter, g.profile.floor, g.profile.ceiling)

	// Min/Max describe the whole day's swing around the mean — not
	// random offsets from the "current" reading.
	minTemp := min(clamp(mean-g.profile.dailyAmplitude+g.rng.IntN(3)-1, g.profile.floor, g.profile.ceiling), current)
	maxTemp := max(clamp(mean+g.profile.dailyAmplitude+g.rng.IntN(3)-1, g.profile.floor, g.profile.ceiling), current)

	// Pick a condition compatible with the temperature + zone + month.
	candidates := candidateConditions(current, g.month, g.zone, g.seasonal)
	condition := candidates[g.rng.IntN(len(candidates))]

	wind := g.wind(condition)
	humidity := g.humidity(condition)
	feelsLike := calculateFeelsLike(current, humidity, wind.Speed)
	pressure := g.pressure(condition)
	visibility := g.visibility(condition, humidity)
	cloudCover := g.cloudCover(condition)
	dewPoint := calculateDewPoint(current, humidity)

	var precipitation *Precipitation
	if precipitationFor(condition) {
		precipitation = g.precipitation(condition, current)
	}

	var airQuality *AirQuality
	if g.request.IncludeAirQuality {
		airQuality = g.airQuality(condition)
	}

	uvIndex := g.uvIndex(condition, cloudCover)
	astronomy := g.astronomy()

	var hourlyForecast []HourlyForecast
	if g.request.IncludeHourly {
		hourlyForecast = g.hourlyForecast(mean, condition)
	}

	alerts := g.alerts(condition, current, wind.Speed)
	description := buildDescription(condition, current, wind, humidity, precipitation)

	startOfDay := time.Date(g.target.Year(), g.target.Month(), g.target.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour)

	return &Response{
		Location:    g.request.Location,
		Coordinates: g.coords,
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
	}
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
// table. For a known city in the southern hemisphere, months are flipped
// (so July reads as January temps). For unknown locations, the lookup uses the
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
