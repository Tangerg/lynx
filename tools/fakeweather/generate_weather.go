package fakeweather

import (
	"math"
)

func (g *reportGenerator) wind(condition string) Wind {
	speed := 5.0 + g.rng.Float64()*15.0
	switch condition {
	case "Stormy", "Blizzard":
		speed += g.rng.Float64() * 30.0
	case "Rainy", "Snowy":
		speed += g.rng.Float64() * 15.0
	case "Sunny", "Clear":
		speed *= 0.6
	}
	switch g.zone {
	case zoneDesert:
		speed += g.rng.Float64() * 10.0
	case zoneOceanic:
		speed += g.rng.Float64() * 8.0
	case zoneAlpine:
		speed += g.rng.Float64() * 12.0
	}
	speed += float64(g.coords.Elevation) * 0.01

	degree := g.rng.IntN(360)
	gust := speed * (1.2 + g.rng.Float64()*0.3)
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

// humidity follows the zone's typical humidity, lifted by
// rainy/foggy conditions and reduced by sunny/dusty ones.
func (g *reportGenerator) humidity(condition string) int {
	base := 50
	switch g.zone {
	case zoneTropical:
		base = 75
		if g.seasonal.monsoonInfluence && monthInRange(g.month, g.seasonal.rainyStart, g.seasonal.rainyEnd) {
			base = 85
		}
	case zoneDesert:
		base = 20
	case zoneMediterranean:
		if g.month >= 6 && g.month <= 9 {
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
		return min(base+20+g.rng.IntN(20), 100)
	case "Snowy", "Blizzard":
		return min(base+15+g.rng.IntN(15), 95)
	case "Cloudy", "Partly Cloudy", "Overcast":
		return base + g.rng.IntN(15)
	case "Sunny", "Clear", "Hot":
		return max(base-20+g.rng.IntN(20), 10)
	case "Dusty", "Hazy":
		return max(base-30+g.rng.IntN(15), 5)
	}
	return base + g.rng.IntN(20) - 10
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

// pressure starts from the elevation-corrected MSL pressure and
// adjusts for the weather (low for storms, high for clear).
func (g *reportGenerator) pressure(condition string) int {
	base := 1013 - g.coords.Elevation/8
	switch condition {
	case "Stormy", "Rainy":
		base += -10 - g.rng.IntN(15)
	case "Sunny", "Clear":
		base += 5 + g.rng.IntN(10)
	case "Cloudy", "Partly Cloudy":
		base += g.rng.IntN(10) - 5
	}
	return base
}

func (g *reportGenerator) visibility(condition string, humidity int) int {
	var base int
	switch condition {
	case "Foggy", "Mist":
		base = g.rng.IntN(2) + 1
	case "Rainy", "Snowy":
		base = 3 + g.rng.IntN(5)
	case "Stormy", "Blizzard":
		base = 1 + g.rng.IntN(3)
	case "Dusty", "Hazy":
		base = 2 + g.rng.IntN(6)
	case "Cloudy":
		base = 8 + g.rng.IntN(7)
	case "Sunny", "Clear":
		base = 15 + g.rng.IntN(35)
	default:
		base = 10 + g.rng.IntN(10)
	}
	if humidity > 85 {
		base = int(float64(base) * 0.7)
	}
	return max(1, base)
}

func (g *reportGenerator) cloudCover(condition string) int {
	switch condition {
	case "Sunny", "Clear":
		return g.rng.IntN(15)
	case "Partly Cloudy":
		return 25 + g.rng.IntN(35)
	case "Cloudy", "Overcast":
		return 75 + g.rng.IntN(25)
	case "Rainy", "Snowy", "Stormy":
		return 90 + g.rng.IntN(10)
	case "Foggy":
		return 100
	}
	return 40 + g.rng.IntN(40)
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

func (g *reportGenerator) precipitation(condition string, temp int) *Precipitation {
	p := &Precipitation{}
	switch {
	case temp < 0:
		p.Type = "snow"
	case temp < 3:
		if g.rng.Float64() < 0.3 {
			p.Type = "sleet"
		} else {
			p.Type = "snow"
		}
	default:
		p.Type = "rain"
	}

	switch condition {
	case "Stormy", "Blizzard":
		p.Probability = 85 + g.rng.IntN(15)
	case "Rainy", "Snowy":
		p.Probability = 60 + g.rng.IntN(30)
	case "Drizzle":
		p.Probability = 40 + g.rng.IntN(30)
	default:
		p.Probability = 30 + g.rng.IntN(40)
	}
	if g.seasonal.monsoonInfluence && monthInRange(g.month, g.seasonal.rainyStart, g.seasonal.rainyEnd) {
		p.Probability = min(100, p.Probability+15)
	}

	switch condition {
	case "Stormy":
		p.Amount = 20.0 + g.rng.Float64()*40.0
		p.Intensity = "heavy"
	case "Rainy":
		p.Amount = 5.0 + g.rng.Float64()*20.0
		if p.Amount > 15 {
			p.Intensity = "moderate"
		} else {
			p.Intensity = "light"
		}
	case "Drizzle":
		p.Amount = 0.5 + g.rng.Float64()*3.0
		p.Intensity = "light"
	case "Snowy", "Blizzard":
		p.Amount = 1.0 + g.rng.Float64()*10.0
		if condition == "Blizzard" {
			p.Intensity = "heavy"
		} else {
			p.Intensity = "moderate"
		}
	default:
		p.Amount = g.rng.Float64() * 5.0
		p.Intensity = "light"
	}
	p.Amount = math.Round(p.Amount*10) / 10
	return p
}

func (g *reportGenerator) airQuality(condition string) *AirQuality {
	aq := &AirQuality{}
	aqi := 50

	if profile, ok := lookupCity(g.request.Location); ok && profile.Polluted {
		aqi = 80 + g.rng.IntN(40)
	}

	switch condition {
	case "Foggy", "Hazy":
		aqi += 40 + g.rng.IntN(30)
	case "Rainy", "Stormy":
		aqi -= 20 + g.rng.IntN(20)
	case "Windy":
		aqi -= 10 + g.rng.IntN(15)
	}
	if g.zone == zoneDesert {
		aqi += 10 + g.rng.IntN(20)
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

	aq.PM25 = int(float64(aq.AQI) * 0.5 * (1 + g.rng.Float64()*0.4))
	aq.PM10 = int(float64(aq.PM25) * 1.5 * (1 + g.rng.Float64()*0.3))
	aq.Ozone = 20 + g.rng.IntN(80)
	return aq
}

func (g *reportGenerator) uvIndex(condition string, cloudCover int) UVIndex {
	absLat := math.Abs(g.coords.Latitude)
	latitudeFactor := 1.0 - absLat/90.0
	var seasonFactor float64
	switch {
	case g.month >= 5 && g.month <= 8:
		seasonFactor = 1.2
	case g.month >= 11 || g.month <= 2:
		seasonFactor = 0.6
	default:
		seasonFactor = 0.9
	}

	value := int(11.0 * latitudeFactor * seasonFactor)
	value -= int(float64(cloudCover) * 0.08)
	switch condition {
	case "Sunny", "Clear":
		value += 1 + g.rng.IntN(2)
	case "Cloudy", "Overcast":
		value -= 2 + g.rng.IntN(2)
	case "Rainy", "Stormy":
		value -= 4 + g.rng.IntN(3)
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

// astronomy uses simplified declination math for sunrise/sunset,
// and a 29.5-day cycle for the moon phase.
func (g *reportGenerator) astronomy() Astronomy {
	dayOfYear := g.target.YearDay()

	declination := 23.45 * math.Sin(2*math.Pi*float64(dayOfYear-81)/365)
	latRad := g.coords.Latitude * math.Pi / 180
	declRad := declination * math.Pi / 180
	cosH := -math.Tan(latRad) * math.Tan(declRad)
	cosH = math.Max(-1, math.Min(1, cosH)) // polar day/night clamp
	hourAngle := math.Acos(cosH)
	daylight := 2 * hourAngle * 12 / math.Pi

	sunrise := formatHM(12 - daylight/2)
	sunset := formatHM(12 + daylight/2)

	phaseIndex := (dayOfYear * 8 / 30) % 8
	moonIllum := int(math.Abs(math.Sin(float64(dayOfYear)*2*math.Pi/29.5)) * 100)

	moonriseOffset := g.rng.IntN(120) - 60
	moonsetOffset := g.rng.IntN(120) - 60
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
