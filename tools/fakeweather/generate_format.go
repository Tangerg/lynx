package fakeweather

import (
	"fmt"
	"math"
	"strings"
	"time"
)

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

func (g *reportGenerator) hourlyForecast(dailyMean int, condition Condition) []HourlyForecast {
	out := make([]HourlyForecast, 24)
	for i := range 24 {
		hour := time.Date(g.target.Year(), g.target.Month(), g.target.Day(), i, 0, 0, 0, time.UTC)

		// Sinusoidal diurnal cycle: hottest at 14:00, coolest at 02:00.
		amp := g.profile.dailyAmplitude
		if g.zone == zoneDesert {
			amp = 12
		}
		variation := int(math.Round(float64(amp) * math.Sin(float64(i-2)*math.Pi/12)))
		hourTemp := clamp(dailyMean+variation+g.rng.IntN(3)-1, g.profile.floor, g.profile.ceiling)

		hourCondition := condition
		if g.rng.Float64() < 0.2 {
			alt := candidateConditions(hourTemp, int(g.target.Month()), g.zone, seasonalPattern{})
			hourCondition = alt[g.rng.IntN(len(alt))]
		}

		precip := 0.0
		if precipitationFor(hourCondition) {
			precip = math.Round(g.rng.Float64()*5.0*10) / 10
		}

		humidity := 50 + g.rng.IntN(30)
		if i >= 22 || i <= 6 {
			humidity = min(humidity+10, 100)
		}

		out[i] = HourlyForecast{
			Time:          hour.Unix(),
			Temperature:   hourTemp,
			Condition:     hourCondition,
			Precipitation: precip,
			Humidity:      humidity,
			WindSpeed:     math.Round((5.0+g.rng.Float64()*15.0)*10) / 10,
		}
	}
	return out
}

func (g *reportGenerator) alerts(condition Condition, temp int, windSpeed float64) []Alert {
	var alerts []Alert
	day := g.target.Add(24 * time.Hour)

	if temp >= 35 {
		severity := AlertSeverityModerate
		if temp >= 40 {
			severity = AlertSeveritySevere
		}
		alerts = append(alerts, Alert{
			Type:        AlertHeat,
			Severity:    severity,
			Title:       "High Temperature Warning",
			Description: fmt.Sprintf("Temperature is expected to reach %d°C. Stay hydrated and avoid prolonged sun exposure.", temp),
			StartTime:   g.target.Unix(),
			EndTime:     day.Unix(),
		})
	}
	if temp <= -10 {
		severity := AlertSeverityModerate
		if temp <= -20 {
			severity = AlertSeveritySevere
		}
		alerts = append(alerts, Alert{
			Type:        AlertCold,
			Severity:    severity,
			Title:       "Extreme Cold Warning",
			Description: fmt.Sprintf("Temperature is expected to drop to %d°C. Dress warmly and limit outdoor exposure.", temp),
			StartTime:   g.target.Unix(),
			EndTime:     day.Unix(),
		})
	}
	if windSpeed >= 50 {
		severity := AlertSeverityModerate
		if windSpeed >= 70 {
			severity = AlertSeveritySevere
		}
		alerts = append(alerts, Alert{
			Type:        AlertWind,
			Severity:    severity,
			Title:       "High Wind Warning",
			Description: fmt.Sprintf("Wind speeds may reach %.1f km/h. Secure loose objects and avoid outdoor activities.", windSpeed),
			StartTime:   g.target.Unix(),
			EndTime:     g.target.Add(12 * time.Hour).Unix(),
		})
	}
	if condition == ConditionStormy {
		alerts = append(alerts, Alert{
			Type:        AlertStorm,
			Severity:    AlertSeveritySevere,
			Title:       "Severe Storm Warning",
			Description: "Severe thunderstorms expected. Stay indoors and avoid travel if possible.",
			StartTime:   g.target.Unix(),
			EndTime:     g.target.Add(6 * time.Hour).Unix(),
		})
	}
	if condition == ConditionBlizzard {
		alerts = append(alerts, Alert{
			Type:        AlertSnow,
			Severity:    AlertSeveritySevere,
			Title:       "Blizzard Warning",
			Description: "Blizzard conditions expected with heavy snow and strong winds. Travel is strongly discouraged.",
			StartTime:   g.target.Unix(),
			EndTime:     g.target.Add(12 * time.Hour).Unix(),
		})
	}
	month := int(g.target.Month())
	if (g.zone == zoneTropical || g.zone == zoneSubtropical) && month >= 6 && month <= 10 && g.rng.Float64() < 0.05 {
		alerts = append(alerts, Alert{
			Type:        AlertTyphoon,
			Severity:    AlertSeverityExtreme,
			Title:       "Typhoon Warning",
			Description: "A typhoon is approaching. Evacuate if instructed by authorities and prepare for extreme weather.",
			StartTime:   g.target.Unix(),
			EndTime:     g.target.Add(48 * time.Hour).Unix(),
		})
	}
	return alerts
}

func buildDescription(condition Condition, temp int, wind Wind, humidity int, precip *Precipitation) string {
	var b strings.Builder

	switch condition {
	case ConditionSunny:
		b.WriteString("Clear skies with abundant sunshine throughout the day.")
	case ConditionPartlyCloudy:
		b.WriteString("Mix of sun and clouds with pleasant weather conditions.")
	case ConditionCloudy:
		b.WriteString("Overcast skies with cloud cover throughout the day.")
	case ConditionRainy:
		b.WriteString("Rainy conditions expected.")
		if precip != nil {
			fmt.Fprintf(&b, " Rainfall amount: %.1f mm. %s intensity.", precip.Amount, precip.Intensity)
		}
	case ConditionStormy:
		b.WriteString("Severe thunderstorms with heavy rain and strong winds. Lightning activity expected.")
	case ConditionSnowy:
		b.WriteString("Snow is expected.")
		if precip != nil {
			fmt.Fprintf(&b, " Snowfall amount: %.1f mm. %s intensity.", precip.Amount, precip.Intensity)
		}
	case ConditionBlizzard:
		b.WriteString("Blizzard conditions with heavy snow and very strong winds. Visibility severely reduced.")
	case ConditionFoggy:
		b.WriteString("Dense fog reducing visibility significantly. Drive with caution.")
	case ConditionHot:
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
