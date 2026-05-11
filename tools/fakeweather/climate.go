package fakeweather

// climateZone is the package's climate-band enum. Used to look up
// monthly base temperature, seasonal pattern, plausible weather
// conditions, and other downstream correlates.
type climateZone int

const (
	zoneTemperate climateZone = iota
	zoneTropical
	zoneSubtropical
	zoneContinental
	zonePolar
	zoneDesert
	zoneMediterranean
	zoneOceanic
	zoneAlpine
)

// seasonalPattern describes a zone's rainfall seasonality. Months are
// 1-based on the *northern hemisphere calendar*; zoneIsSouthern() is
// applied where appropriate.
type seasonalPattern struct {
	rainyStart       int // inclusive (1..12), 0 = no rainy season
	rainyEnd         int // inclusive
	monsoonInfluence bool
	drySeason        bool
}

// climateProfile bundles a zone's monthly mean temperature table plus
// realistic floor/ceiling bounds. Floors prevent jitter+elevation
// from producing impossible values (e.g., a 30°C summer reading
// dropping below 0°C). Index is month-1 (0..11).
type climateProfile struct {
	mean [12]int // monthly mean (°C)
	// dailyAmplitude is the typical Mean→Max swing in °C; Min is
	// symmetric around mean. Day-of-year jitter on top is ±2°C.
	dailyAmplitude int
	// floor and ceiling clamp the final synthesized temperature so
	// jitter+elevation can never produce physically absurd values.
	floor   int // °C lower bound (regardless of month)
	ceiling int // °C upper bound (regardless of month)
}

// climateProfiles is the per-zone table used by every temperature
// derivation. Numbers are deliberately conservative — a synthesized
// "typical" climate, not record extremes.
var climateProfiles = map[climateZone]climateProfile{
	zoneTemperate: {
		mean:           [12]int{5, 7, 12, 18, 23, 28, 30, 29, 24, 18, 12, 7},
		dailyAmplitude: 6,
		floor:          -15, ceiling: 40,
	},
	zoneTropical: {
		mean:           [12]int{27, 27, 28, 29, 29, 28, 28, 28, 28, 28, 27, 27},
		dailyAmplitude: 4,
		floor:          18, ceiling: 38,
	},
	zoneSubtropical: {
		mean:           [12]int{10, 12, 16, 22, 26, 30, 32, 31, 28, 22, 16, 11},
		dailyAmplitude: 6,
		floor:          -5, ceiling: 40,
	},
	zoneContinental: {
		mean:           [12]int{-5, -2, 5, 14, 21, 26, 28, 26, 20, 12, 3, -3},
		dailyAmplitude: 8,
		floor:          -35, ceiling: 38,
	},
	zonePolar: {
		mean:           [12]int{-25, -22, -15, -8, -2, 3, 5, 4, -1, -10, -18, -23},
		dailyAmplitude: 4,
		floor:          -55, ceiling: 12,
	},
	zoneDesert: {
		mean:           [12]int{15, 18, 22, 28, 35, 40, 42, 41, 37, 30, 22, 16},
		dailyAmplitude: 12, // characteristic large diurnal swing
		floor:          0, ceiling: 50,
	},
	zoneMediterranean: {
		mean:           [12]int{12, 13, 15, 18, 22, 27, 30, 30, 26, 21, 16, 13},
		dailyAmplitude: 7,
		floor:          -5, ceiling: 42,
	},
	zoneOceanic: {
		mean:           [12]int{7, 8, 10, 13, 16, 19, 21, 21, 18, 14, 10, 8},
		dailyAmplitude: 5,
		floor:          -10, ceiling: 32,
	},
	zoneAlpine: {
		mean:           [12]int{-5, -3, 2, 8, 13, 17, 19, 18, 14, 9, 2, -3},
		dailyAmplitude: 8,
		floor:          -25, ceiling: 28,
	},
}

// seasonalPatterns is the per-zone rainfall pattern. Only zones with
// a meaningful pattern are listed; the default zero value is fine for
// the rest.
var seasonalPatterns = map[climateZone]seasonalPattern{
	zoneTropical:      {rainyStart: 5, rainyEnd: 10, monsoonInfluence: true},
	zoneSubtropical:   {rainyStart: 4, rainyEnd: 9, monsoonInfluence: true},
	zoneMediterranean: {rainyStart: 11, rainyEnd: 3, drySeason: true},
	zoneDesert:        {drySeason: true},
}

// identifyClimateZone returns the zone for the requested location.
// All lookups go through the data tables in cities.go — this
// function holds no city/region names of its own. Order:
//
//  1. Regional patterns ([lookupRegion]) — most specific intent
//     ("antarctica research base" → polar).
//  2. Known cities ([lookupCity]) — gazetteer entries.
//  3. zoneTemperate fallback — also signals "treat as northern
//     hemisphere" downstream (avoids the "summer = winter" surprise
//     earlier versions produced for unknown southern-hemisphere
//     locations).
func identifyClimateZone(location string) climateZone {
	if zone, ok := lookupRegion(location); ok {
		return zone
	}
	if profile, ok := lookupCity(location); ok {
		return profile.Zone
	}
	return zoneTemperate
}

// candidateConditions returns the list of weather conditions that are
// plausible for the given (mean temp, month, zone, seasonal pattern).
// The caller picks one uniformly at random.
func candidateConditions(temp int, month int, zone climateZone, seasonal seasonalPattern) []string {
	isSummer := month >= 6 && month <= 8
	isWinter := month == 12 || month <= 2
	isRainy := seasonal.monsoonInfluence && monthInRange(month, seasonal.rainyStart, seasonal.rainyEnd)

	switch zone {
	case zoneTropical:
		if isRainy {
			return []string{"Rainy", "Stormy", "Partly Cloudy", "Humid", "Drizzle"}
		}
		return []string{"Partly Cloudy", "Humid", "Sunny", "Rainy"}

	case zoneDesert:
		if temp > 38 {
			return []string{"Sunny", "Hot", "Clear", "Dusty", "Hazy"}
		}
		return []string{"Sunny", "Clear", "Partly Cloudy", "Dusty"}

	case zoneMediterranean:
		if isSummer {
			return []string{"Sunny", "Clear", "Hot", "Partly Cloudy"}
		}
		return []string{"Rainy", "Cloudy", "Partly Cloudy", "Clear", "Drizzle"}

	case zonePolar:
		if temp < -15 {
			return []string{"Snowy", "Blizzard", "Cloudy", "Freezing", "Clear"}
		}
		return []string{"Snowy", "Cloudy", "Clear", "Cold", "Overcast"}

	case zoneContinental:
		switch {
		case temp < -5:
			return []string{"Snowy", "Cloudy", "Clear", "Cold", "Blizzard"}
		case temp > 28 && isSummer:
			return []string{"Sunny", "Hot", "Stormy", "Partly Cloudy", "Clear"}
		}
		return []string{"Sunny", "Partly Cloudy", "Cloudy", "Clear", "Rainy"}

	case zoneOceanic:
		if isWinter {
			return []string{"Rainy", "Cloudy", "Drizzle", "Overcast", "Foggy"}
		}
		return []string{"Partly Cloudy", "Cloudy", "Sunny", "Rainy", "Clear"}

	case zoneAlpine:
		if temp < 5 {
			return []string{"Snowy", "Cloudy", "Clear", "Cold", "Windy"}
		}
		return []string{"Partly Cloudy", "Sunny", "Clear", "Cloudy", "Rainy"}
	}

	// zoneTemperate (default)
	switch {
	case temp < 0:
		return []string{"Snowy", "Cloudy", "Clear", "Cold", "Freezing"}
	case temp < 10:
		return []string{"Cloudy", "Clear", "Rainy", "Foggy", "Drizzle"}
	case temp < 25:
		return []string{"Sunny", "Partly Cloudy", "Cloudy", "Clear", "Mild"}
	}
	if isSummer {
		return []string{"Sunny", "Partly Cloudy", "Rainy", "Stormy", "Hot"}
	}
	return []string{"Sunny", "Hot", "Partly Cloudy", "Clear"}
}

// monthInRange returns whether month falls within the inclusive
// [start, end] window, handling wrap-around (e.g., Mediterranean
// rainy season Nov..Mar).
func monthInRange(month, start, end int) bool {
	if start == 0 && end == 0 {
		return false
	}
	if start <= end {
		return month >= start && month <= end
	}
	return month >= start || month <= end
}
