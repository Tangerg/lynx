package fakeweather

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
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
		return time.Time{}, fmt.Errorf("invalid date %q (want YYYY-MM-DD): %w", s, err)
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
func generateWind(condition string, zone climateZone, coords Coordinates, rng *rand.Rand) Wind {
	speed := 5.0 + rng.Float64()*15.0
	switch condition {
	case "Stormy", "Blizzard":
		speed += rng.Float64() * 30.0
	case "Rainy", "Snowy":
		speed += rng.Float64() * 15.0
	case "Sunny", "Clear":
		speed *= 0.6
	}
	switch zone {
	case zoneDesert:
		speed += rng.Float64() * 10.0
	case zoneOceanic:
		speed += rng.Float64() * 8.0
	case zoneAlpine:
		speed += rng.Float64() * 12.0
	}
	speed += float64(coords.Elevation) * 0.01

	degree := rng.IntN(360)
	gust := speed * (1.2 + rng.Float64()*0.3)
	return Wind{
		Speed:     math.Round(speed*10) / 10,
		Unit:      "km/h",
		Direction: directionFromDegree(degree),
		Degree:    degree,
		Gust:      math.Round(gust*10) / 10,
	}
}

func directionFromDegree(deg int) string {
	directions := []string{
		"North", "North-North-East", "North-East", "East-North-East",
		"East", "East-South-East", "South-East", "South-South-East",
		"South", "South-South-West", "South-West", "West-South-West",
		"West", "West-North-West", "North-West", "North-North-West",
	}
	return directions[int(math.Round(float64(deg)/22.5))%16]
}

// generateHumidity follows the zone's typical humidity, lifted by
// rainy/foggy conditions and reduced by sunny/dusty ones.
func generateHumidity(condition string, zone climateZone, month int, seasonal seasonalPattern, rng *rand.Rand) int {
	base := 50
	switch zone {
	case zoneTropical:
		base = 75
		if seasonal.monsoonInfluence && monthInRange(month, seasonal.rainyStart, seasonal.rainyEnd) {
			base = 85
		}
	case zoneDesert:
		base = 20
	case zoneMediterranean:
		if month >= 6 && month <= 9 {
			base = 45
		} else {
			base = 65
		}
	case zonePolar:
		base = 70
	case zoneOceanic:
		base = 75
	case zoneAlpine:
		base = 60
	case zoneContinental:
		base = 55
	}

	switch condition {
	case "Rainy", "Stormy", "Foggy", "Humid", "Drizzle":
		return min(base+20+rng.IntN(20), 100)
	case "Snowy", "Blizzard":
		return min(base+15+rng.IntN(15), 95)
	case "Cloudy", "Partly Cloudy", "Overcast":
		return base + rng.IntN(15)
	case "Sunny", "Clear", "Hot":
		return max(base-20+rng.IntN(20), 10)
	case "Dusty", "Hazy":
		return max(base-30+rng.IntN(15), 5)
	}
	return base + rng.IntN(20) - 10
}

// calculateFeelsLike applies wind-chill (cold + windy) and heat-index
// (hot + humid) corrections; falls back to the raw temp otherwise.
func calculateFeelsLike(temp, humidity int, windSpeed float64) int {
	t := float64(temp)
	feels := t

	if temp < 10 && windSpeed > 4.8 {
		feels = 13.12 + 0.6215*t - 11.37*math.Pow(windSpeed, 0.16) +
			0.3965*t*math.Pow(windSpeed, 0.16)
	}

	if temp > 27 && humidity > 40 {
		rh := float64(humidity)
		feels = -8.78469475556 + 1.61139411*t + 2.33854883889*rh -
			0.14611605*t*rh - 0.012308094*t*t - 0.0164248277778*rh*rh +
			0.002211732*t*t*rh + 0.00072546*t*rh*rh - 0.000003582*t*t*rh*rh
	}

	return int(math.Round(feels))
}

// generatePressure starts from the elevation-corrected MSL pressure and
// adjusts for the weather (low for storms, high for clear).
func generatePressure(elevation int, condition string, rng *rand.Rand) int {
	base := 1013 - elevation/8
	switch condition {
	case "Stormy", "Rainy":
		base += -10 - rng.IntN(15)
	case "Sunny", "Clear":
		base += 5 + rng.IntN(10)
	case "Cloudy", "Partly Cloudy":
		base += rng.IntN(10) - 5
	}
	return base
}

func generateVisibility(condition string, humidity int, rng *rand.Rand) int {
	var base int
	switch condition {
	case "Foggy", "Mist":
		base = rng.IntN(2) + 1
	case "Rainy", "Snowy":
		base = 3 + rng.IntN(5)
	case "Stormy", "Blizzard":
		base = 1 + rng.IntN(3)
	case "Dusty", "Hazy":
		base = 2 + rng.IntN(6)
	case "Cloudy":
		base = 8 + rng.IntN(7)
	case "Sunny", "Clear":
		base = 15 + rng.IntN(35)
	default:
		base = 10 + rng.IntN(10)
	}
	if humidity > 85 {
		base = int(float64(base) * 0.7)
	}
	return max(1, base)
}

func generateCloudCover(condition string, rng *rand.Rand) int {
	switch condition {
	case "Sunny", "Clear":
		return rng.IntN(15)
	case "Partly Cloudy":
		return 25 + rng.IntN(35)
	case "Cloudy", "Overcast":
		return 75 + rng.IntN(25)
	case "Rainy", "Snowy", "Stormy":
		return 90 + rng.IntN(10)
	case "Foggy":
		return 100
	}
	return 40 + rng.IntN(40)
}

// calculateDewPoint applies the Magnus formula. Returns the dew point
// in °C, rounded to int.
func calculateDewPoint(temp, humidity int) int {
	const a = 17.27
	const b = 237.7
	t := float64(temp)
	rh := float64(humidity) / 100.0
	if rh <= 0 {
		return temp
	}
	alpha := (a*t)/(b+t) + math.Log(rh)
	return int(math.Round((b * alpha) / (a - alpha)))
}

func precipitationFor(condition string) bool {
	switch condition {
	case "Rainy", "Snowy", "Stormy", "Blizzard", "Sleet", "Drizzle":
		return true
	}
	return false
}

func generatePrecipitation(condition string, temp int, month int, seasonal seasonalPattern, rng *rand.Rand) *Precipitation {
	p := &Precipitation{}
	switch {
	case temp < 0:
		p.Type = "snow"
	case temp < 3:
		if rng.Float64() < 0.3 {
			p.Type = "sleet"
		} else {
			p.Type = "snow"
		}
	default:
		p.Type = "rain"
	}

	switch condition {
	case "Stormy", "Blizzard":
		p.Probability = 85 + rng.IntN(15)
	case "Rainy", "Snowy":
		p.Probability = 60 + rng.IntN(30)
	case "Drizzle":
		p.Probability = 40 + rng.IntN(30)
	default:
		p.Probability = 30 + rng.IntN(40)
	}
	if seasonal.monsoonInfluence && monthInRange(month, seasonal.rainyStart, seasonal.rainyEnd) {
		p.Probability = min(100, p.Probability+15)
	}

	switch condition {
	case "Stormy":
		p.Amount = 20.0 + rng.Float64()*40.0
		p.Intensity = "heavy"
	case "Rainy":
		p.Amount = 5.0 + rng.Float64()*20.0
		if p.Amount > 15 {
			p.Intensity = "moderate"
		} else {
			p.Intensity = "light"
		}
	case "Drizzle":
		p.Amount = 0.5 + rng.Float64()*3.0
		p.Intensity = "light"
	case "Snowy", "Blizzard":
		p.Amount = 1.0 + rng.Float64()*10.0
		if condition == "Blizzard" {
			p.Intensity = "heavy"
		} else {
			p.Intensity = "moderate"
		}
	default:
		p.Amount = rng.Float64() * 5.0
		p.Intensity = "light"
	}
	p.Amount = math.Round(p.Amount*10) / 10
	return p
}

func generateAirQuality(location, condition string, zone climateZone, rng *rand.Rand) *AirQuality {
	aq := &AirQuality{}
	aqi := 50

	if profile, ok := lookupCity(location); ok && profile.Polluted {
		aqi = 80 + rng.IntN(40)
	}

	switch condition {
	case "Foggy", "Hazy":
		aqi += 40 + rng.IntN(30)
	case "Rainy", "Stormy":
		aqi -= 20 + rng.IntN(20)
	case "Windy":
		aqi -= 10 + rng.IntN(15)
	}
	if zone == zoneDesert {
		aqi += 10 + rng.IntN(20)
	}

	aq.AQI = clamp(aqi, 0, 500)
	switch {
	case aq.AQI <= 50:
		aq.Level = "Good"
		aq.Description = "Air quality is satisfactory, and air pollution poses little or no risk."
	case aq.AQI <= 100:
		aq.Level = "Moderate"
		aq.Description = "Air quality is acceptable. There may be a risk for some people sensitive to air pollution."
	case aq.AQI <= 150:
		aq.Level = "Unhealthy for Sensitive Groups"
		aq.Description = "Members of sensitive groups may experience health effects."
	case aq.AQI <= 200:
		aq.Level = "Unhealthy"
		aq.Description = "Some members of the general public may experience health effects."
	case aq.AQI <= 300:
		aq.Level = "Very Unhealthy"
		aq.Description = "Health alert: the risk of health effects is increased for everyone."
	default:
		aq.Level = "Hazardous"
		aq.Description = "Health warning of emergency conditions: everyone is more likely to be affected."
	}

	aq.PM25 = int(float64(aq.AQI) * 0.5 * (1 + rng.Float64()*0.4))
	aq.PM10 = int(float64(aq.PM25) * 1.5 * (1 + rng.Float64()*0.3))
	aq.Ozone = 20 + rng.IntN(80)
	return aq
}

func generateUVIndex(month int, latitude float64, condition string, cloudCover int, rng *rand.Rand) UVIndex {
	absLat := math.Abs(latitude)
	latitudeFactor := 1.0 - absLat/90.0
	var seasonFactor float64
	switch {
	case month >= 5 && month <= 8:
		seasonFactor = 1.2
	case month >= 11 || month <= 2:
		seasonFactor = 0.6
	default:
		seasonFactor = 0.9
	}

	value := int(11.0 * latitudeFactor * seasonFactor)
	value -= int(float64(cloudCover) * 0.08)
	switch condition {
	case "Sunny", "Clear":
		value += 1 + rng.IntN(2)
	case "Cloudy", "Overcast":
		value -= 2 + rng.IntN(2)
	case "Rainy", "Stormy":
		value -= 4 + rng.IntN(3)
	}
	value = clamp(value, 0, 11)

	uv := UVIndex{Value: value}
	switch {
	case value <= 2:
		uv.Level = "Low"
		uv.Description = "No protection required. You can safely stay outside."
	case value <= 5:
		uv.Level = "Moderate"
		uv.Description = "Seek shade during midday hours. Wear sunscreen and a hat."
	case value <= 7:
		uv.Level = "High"
		uv.Description = "Protection essential. Seek shade during midday hours."
	case value <= 10:
		uv.Level = "Very High"
		uv.Description = "Extra protection needed. Avoid sun exposure during midday."
	default:
		uv.Level = "Extreme"
		uv.Description = "Take all precautions. Unprotected skin will burn quickly."
	}
	return uv
}

var moonPhases = []string{
	"New Moon", "Waxing Crescent", "First Quarter", "Waxing Gibbous",
	"Full Moon", "Waning Gibbous", "Last Quarter", "Waning Crescent",
}

// generateAstronomy uses simplified declination math for sunrise/sunset,
// and a 29.5-day cycle for the moon phase.
func generateAstronomy(date time.Time, coords Coordinates, rng *rand.Rand) Astronomy {
	dayOfYear := date.YearDay()

	declination := 23.45 * math.Sin(2*math.Pi*float64(dayOfYear-81)/365)
	latRad := coords.Latitude * math.Pi / 180
	declRad := declination * math.Pi / 180
	cosH := -math.Tan(latRad) * math.Tan(declRad)
	cosH = math.Max(-1, math.Min(1, cosH)) // polar day/night clamp
	hourAngle := math.Acos(cosH)
	daylight := 2 * hourAngle * 12 / math.Pi

	sunrise := formatHM(12 - daylight/2)
	sunset := formatHM(12 + daylight/2)

	phaseIndex := (dayOfYear * 8 / 30) % 8
	moonIllum := int(math.Abs(math.Sin(float64(dayOfYear)*2*math.Pi/29.5)) * 100)

	moonriseOffset := rng.IntN(120) - 60
	moonsetOffset := rng.IntN(120) - 60
	moonrise := formatHM(12 - daylight/2 + float64(moonriseOffset)/60.0)
	moonset := formatHM(12 + daylight/2 + float64(moonsetOffset)/60.0)

	return Astronomy{
		Sunrise:          sunrise,
		Sunset:           sunset,
		Moonrise:         moonrise,
		Moonset:          moonset,
		MoonPhase:        moonPhases[phaseIndex],
		MoonIllumination: moonIllum,
	}
}

func formatHM(hours float64) string {
	if math.IsNaN(hours) || math.IsInf(hours, 0) {
		return "--:--"
	}
	h := int(math.Floor(hours))
	m := int((hours - math.Floor(hours)) * 60)
	if h < 0 {
		h += 24
	}
	h %= 24
	if m < 0 {
		m += 60
	}
	return fmt.Sprintf("%02d:%02d", h, m)
}

func generateHourlyForecast(date time.Time, dailyMean int, condition string, zone climateZone, profile climateProfile, rng *rand.Rand) []HourlyForecast {
	out := make([]HourlyForecast, 24)
	for i := range 24 {
		hour := time.Date(date.Year(), date.Month(), date.Day(), i, 0, 0, 0, time.UTC)

		// Sinusoidal diurnal cycle: hottest at 14:00, coolest at 02:00.
		amp := profile.dailyAmplitude
		if zone == zoneDesert {
			amp = 12
		}
		variation := int(math.Round(float64(amp) * math.Sin(float64(i-2)*math.Pi/12)))
		hourTemp := clamp(dailyMean+variation+rng.IntN(3)-1, profile.floor, profile.ceiling)

		hourCondition := condition
		if rng.Float64() < 0.2 {
			alt := candidateConditions(hourTemp, int(date.Month()), zone, seasonalPattern{})
			hourCondition = alt[rng.IntN(len(alt))]
		}

		precip := 0.0
		if precipitationFor(hourCondition) {
			precip = math.Round(rng.Float64()*5.0*10) / 10
		}

		humidity := 50 + rng.IntN(30)
		if i >= 22 || i <= 6 {
			humidity = min(humidity+10, 100)
		}

		out[i] = HourlyForecast{
			Time:          hour.Unix(),
			Temperature:   hourTemp,
			Condition:     hourCondition,
			Precipitation: precip,
			Humidity:      humidity,
			WindSpeed:     math.Round((5.0+rng.Float64()*15.0)*10) / 10,
		}
	}
	return out
}

func generateAlerts(condition string, temp int, windSpeed float64, zone climateZone, date time.Time, rng *rand.Rand) []Alert {
	var alerts []Alert
	day := date.Add(24 * time.Hour)

	if temp >= 35 {
		severity := "moderate"
		if temp >= 40 {
			severity = "severe"
		}
		alerts = append(alerts, Alert{
			Type:        "heat",
			Severity:    severity,
			Title:       "High Temperature Warning",
			Description: fmt.Sprintf("Temperature is expected to reach %d°C. Stay hydrated and avoid prolonged sun exposure.", temp),
			StartTime:   date.Unix(),
			EndTime:     day.Unix(),
		})
	}
	if temp <= -10 {
		severity := "moderate"
		if temp <= -20 {
			severity = "severe"
		}
		alerts = append(alerts, Alert{
			Type:        "cold",
			Severity:    severity,
			Title:       "Extreme Cold Warning",
			Description: fmt.Sprintf("Temperature is expected to drop to %d°C. Dress warmly and limit outdoor exposure.", temp),
			StartTime:   date.Unix(),
			EndTime:     day.Unix(),
		})
	}
	if windSpeed >= 50 {
		severity := "moderate"
		if windSpeed >= 70 {
			severity = "severe"
		}
		alerts = append(alerts, Alert{
			Type:        "wind",
			Severity:    severity,
			Title:       "High Wind Warning",
			Description: fmt.Sprintf("Wind speeds may reach %.1f km/h. Secure loose objects and avoid outdoor activities.", windSpeed),
			StartTime:   date.Unix(),
			EndTime:     date.Add(12 * time.Hour).Unix(),
		})
	}
	if condition == "Stormy" {
		alerts = append(alerts, Alert{
			Type:        "storm",
			Severity:    "severe",
			Title:       "Severe Storm Warning",
			Description: "Severe thunderstorms expected. Stay indoors and avoid travel if possible.",
			StartTime:   date.Unix(),
			EndTime:     date.Add(6 * time.Hour).Unix(),
		})
	}
	if condition == "Blizzard" {
		alerts = append(alerts, Alert{
			Type:        "snow",
			Severity:    "severe",
			Title:       "Blizzard Warning",
			Description: "Blizzard conditions expected with heavy snow and strong winds. Travel is strongly discouraged.",
			StartTime:   date.Unix(),
			EndTime:     date.Add(12 * time.Hour).Unix(),
		})
	}
	month := int(date.Month())
	if (zone == zoneTropical || zone == zoneSubtropical) && month >= 6 && month <= 10 && rng.Float64() < 0.05 {
		alerts = append(alerts, Alert{
			Type:        "typhoon",
			Severity:    "extreme",
			Title:       "Typhoon Warning",
			Description: "A typhoon is approaching. Evacuate if instructed by authorities and prepare for extreme weather.",
			StartTime:   date.Unix(),
			EndTime:     date.Add(48 * time.Hour).Unix(),
		})
	}
	return alerts
}

func buildDescription(condition string, temp int, wind Wind, humidity int, precip *Precipitation) string {
	var b strings.Builder

	switch condition {
	case "Sunny":
		b.WriteString("Clear skies with abundant sunshine throughout the day.")
	case "Partly Cloudy":
		b.WriteString("Mix of sun and clouds with pleasant weather conditions.")
	case "Cloudy":
		b.WriteString("Overcast skies with cloud cover throughout the day.")
	case "Rainy":
		b.WriteString("Rainy conditions expected.")
		if precip != nil {
			fmt.Fprintf(&b, " Rainfall amount: %.1f mm. %s intensity.", precip.Amount, precip.Intensity)
		}
	case "Stormy":
		b.WriteString("Severe thunderstorms with heavy rain and strong winds. Lightning activity expected.")
	case "Snowy":
		b.WriteString("Snow is expected.")
		if precip != nil {
			fmt.Fprintf(&b, " Snowfall amount: %.1f mm. %s intensity.", precip.Amount, precip.Intensity)
		}
	case "Blizzard":
		b.WriteString("Blizzard conditions with heavy snow and very strong winds. Visibility severely reduced.")
	case "Foggy":
		b.WriteString("Dense fog reducing visibility significantly. Drive with caution.")
	case "Hot":
		b.WriteString("Hot and sunny conditions. Take precautions against heat.")
	default:
		fmt.Fprintf(&b, "%s weather conditions expected.", condition)
	}

	switch {
	case temp < 0:
		b.WriteString(" Freezing temperatures.")
	case temp > 30:
		b.WriteString(" High temperatures.")
	}

	switch {
	case wind.Speed > 30:
		fmt.Fprintf(&b, " Strong winds from the %s at %.1f km/h.", strings.ToLower(wind.Direction), wind.Speed)
	case wind.Speed > 15:
		fmt.Fprintf(&b, " Moderate winds from the %s.", strings.ToLower(wind.Direction))
	}

	switch {
	case humidity > 80:
		b.WriteString(" High humidity making it feel muggy.")
	case humidity < 30:
		b.WriteString(" Low humidity with dry air.")
	}
	return b.String()
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
