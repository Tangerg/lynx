package fakeweather

import (
	"math"
)

func (g *reportGenerator) wind(condition Condition) Wind {
	speed := 5.0 + g.rng.Float64()*15.0
	switch condition {
	case ConditionStormy, ConditionBlizzard:
		speed += g.rng.Float64() * 30.0
	case ConditionRainy, ConditionSnowy:
		speed += g.rng.Float64() * 15.0
	case ConditionSunny, ConditionClear:
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
func (g *reportGenerator) humidity(condition Condition) int {
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
	case ConditionRainy, ConditionStormy, ConditionFoggy, ConditionHumid, ConditionDrizzle:
		return min(base+20+g.rng.IntN(20), 100)
	case ConditionSnowy, ConditionBlizzard:
		return min(base+15+g.rng.IntN(15), 95)
	case ConditionCloudy, ConditionPartlyCloudy, ConditionOvercast:
		return base + g.rng.IntN(15)
	case ConditionSunny, ConditionClear, ConditionHot:
		return max(base-20+g.rng.IntN(20), 10)
	case ConditionDusty, ConditionHazy:
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
func (g *reportGenerator) pressure(condition Condition) int {
	base := 1013 - g.coords.Elevation/8
	switch condition {
	case ConditionStormy, ConditionRainy:
		base += -10 - g.rng.IntN(15)
	case ConditionSunny, ConditionClear:
		base += 5 + g.rng.IntN(10)
	case ConditionCloudy, ConditionPartlyCloudy:
		base += g.rng.IntN(10) - 5
	}
	return base
}

func (g *reportGenerator) visibility(condition Condition, humidity int) int {
	var base int
	switch condition {
	case ConditionFoggy:
		base = g.rng.IntN(2) + 1
	case ConditionRainy, ConditionSnowy:
		base = 3 + g.rng.IntN(5)
	case ConditionStormy, ConditionBlizzard:
		base = 1 + g.rng.IntN(3)
	case ConditionDusty, ConditionHazy:
		base = 2 + g.rng.IntN(6)
	case ConditionCloudy:
		base = 8 + g.rng.IntN(7)
	case ConditionSunny, ConditionClear:
		base = 15 + g.rng.IntN(35)
	default:
		base = 10 + g.rng.IntN(10)
	}
	if humidity > 85 {
		base = int(float64(base) * 0.7)
	}
	return max(1, base)
}

func (g *reportGenerator) cloudCover(condition Condition) int {
	switch condition {
	case ConditionSunny, ConditionClear:
		return g.rng.IntN(15)
	case ConditionPartlyCloudy:
		return 25 + g.rng.IntN(35)
	case ConditionCloudy, ConditionOvercast:
		return 75 + g.rng.IntN(25)
	case ConditionRainy, ConditionSnowy, ConditionStormy:
		return 90 + g.rng.IntN(10)
	case ConditionFoggy:
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

func precipitationFor(condition Condition) bool {
	switch condition {
	case ConditionRainy, ConditionSnowy, ConditionStormy, ConditionBlizzard, ConditionDrizzle:
		return true
	}
	return false
}

func (g *reportGenerator) precipitation(condition Condition, temp int) *Precipitation {
	p := &Precipitation{}
	switch {
	case temp < 0:
		p.Type = PrecipitationSnow
	case temp < 3:
		if g.rng.Float64() < 0.3 {
			p.Type = PrecipitationSleet
		} else {
			p.Type = PrecipitationSnow
		}
	default:
		p.Type = PrecipitationRain
	}

	switch condition {
	case ConditionStormy, ConditionBlizzard:
		p.Probability = 85 + g.rng.IntN(15)
	case ConditionRainy, ConditionSnowy:
		p.Probability = 60 + g.rng.IntN(30)
	case ConditionDrizzle:
		p.Probability = 40 + g.rng.IntN(30)
	default:
		p.Probability = 30 + g.rng.IntN(40)
	}
	if g.seasonal.monsoonInfluence && monthInRange(g.month, g.seasonal.rainyStart, g.seasonal.rainyEnd) {
		p.Probability = min(100, p.Probability+15)
	}

	switch condition {
	case ConditionStormy:
		p.Amount = 20.0 + g.rng.Float64()*40.0
		p.Intensity = PrecipitationHeavy
	case ConditionRainy:
		p.Amount = 5.0 + g.rng.Float64()*20.0
		if p.Amount > 15 {
			p.Intensity = PrecipitationModerate
		} else {
			p.Intensity = PrecipitationLight
		}
	case ConditionDrizzle:
		p.Amount = 0.5 + g.rng.Float64()*3.0
		p.Intensity = PrecipitationLight
	case ConditionSnowy, ConditionBlizzard:
		p.Amount = 1.0 + g.rng.Float64()*10.0
		if condition == ConditionBlizzard {
			p.Intensity = PrecipitationHeavy
		} else {
			p.Intensity = PrecipitationModerate
		}
	default:
		p.Amount = g.rng.Float64() * 5.0
		p.Intensity = PrecipitationLight
	}
	p.Amount = math.Round(p.Amount*10) / 10
	return p
}

func (g *reportGenerator) airQuality(condition Condition) *AirQuality {
	aq := &AirQuality{}
	aqi := 50

	if profile, ok := lookupCity(g.request.Location); ok && profile.Polluted {
		aqi = 80 + g.rng.IntN(40)
	}

	switch condition {
	case ConditionFoggy, ConditionHazy:
		aqi += 40 + g.rng.IntN(30)
	case ConditionRainy, ConditionStormy:
		aqi -= 20 + g.rng.IntN(20)
	case ConditionWindy:
		aqi -= 10 + g.rng.IntN(15)
	}
	if g.zone == zoneDesert {
		aqi += 10 + g.rng.IntN(20)
	}

	aq.AQI = clamp(aqi, 0, 500)
	switch {
	case aq.AQI <= 50:
		aq.Level = AirQualityGood
		aq.Description = "Air quality is satisfactory, and air pollution poses little or no risk."
	case aq.AQI <= 100:
		aq.Level = AirQualityModerate
		aq.Description = "Air quality is acceptable. There may be a risk for some people sensitive to air pollution."
	case aq.AQI <= 150:
		aq.Level = AirQualityUnhealthyForSensitiveGroups
		aq.Description = "Members of sensitive groups may experience health effects."
	case aq.AQI <= 200:
		aq.Level = AirQualityUnhealthy
		aq.Description = "Some members of the general public may experience health effects."
	case aq.AQI <= 300:
		aq.Level = AirQualityVeryUnhealthy
		aq.Description = "Health alert: the risk of health effects is increased for everyone."
	default:
		aq.Level = AirQualityHazardous
		aq.Description = "Health warning of emergency conditions: everyone is more likely to be affected."
	}

	aq.PM25 = int(float64(aq.AQI) * 0.5 * (1 + g.rng.Float64()*0.4))
	aq.PM10 = int(float64(aq.PM25) * 1.5 * (1 + g.rng.Float64()*0.3))
	aq.Ozone = 20 + g.rng.IntN(80)
	return aq
}

func (g *reportGenerator) uvIndex(condition Condition, cloudCover int) UVIndex {
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
	case ConditionSunny, ConditionClear:
		value += 1 + g.rng.IntN(2)
	case ConditionCloudy, ConditionOvercast:
		value -= 2 + g.rng.IntN(2)
	case ConditionRainy, ConditionStormy:
		value -= 4 + g.rng.IntN(3)
	}
	value = clamp(value, 0, 11)

	uv := UVIndex{Value: value}
	switch {
	case value <= 2:
		uv.Level = UVLow
		uv.Description = "No protection required. You can safely stay outside."
	case value <= 5:
		uv.Level = UVModerate
		uv.Description = "Seek shade during midday hours. Wear sunscreen and a hat."
	case value <= 7:
		uv.Level = UVHigh
		uv.Description = "Protection essential. Seek shade during midday hours."
	case value <= 10:
		uv.Level = UVVeryHigh
		uv.Description = "Extra protection needed. Avoid sun exposure during midday."
	default:
		uv.Level = UVExtreme
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
