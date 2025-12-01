package fakeweatherquery

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"time"
)

// Temperature temperature information
type Temperature struct {
	Value     int    `json:"value"`
	Unit      string `json:"unit"`
	FeelsLike int    `json:"feels_like"` // feels like temperature
	Min       int    `json:"min"`        // minimum temperature
	Max       int    `json:"max"`        // maximum temperature
}

func (t *Temperature) Condition() string {
	if t.Unit == "Celsius" {
		switch {
		case t.Value < -10:
			return "Extremely Cold"
		case t.Value < 0:
			return "Freezing"
		case t.Value < 10:
			return "Cold"
		case t.Value < 15:
			return "Cool"
		case t.Value < 20:
			return "Mild"
		case t.Value < 28:
			return "Comfortable"
		case t.Value < 35:
			return "Hot"
		case t.Value < 40:
			return "Very Hot"
		default:
			return "Extremely Hot"
		}
	}
	return "Unknown"
}

// Wind wind information
type Wind struct {
	Speed     float64 `json:"speed"`
	Unit      string  `json:"unit"`
	Direction string  `json:"direction"`
	Degree    int     `json:"degree"` // wind direction in degrees
	Gust      float64 `json:"gust"`   // wind gust speed
}

// Precipitation precipitation information
type Precipitation struct {
	Type        string  `json:"type"`        // rain, snow, sleet
	Probability int     `json:"probability"` // precipitation probability 0-100
	Amount      float64 `json:"amount"`      // precipitation amount in mm
	Intensity   string  `json:"intensity"`   // light, moderate, heavy
}

// AirQuality air quality information
type AirQuality struct {
	AQI         int    `json:"aqi"`   // Air Quality Index
	Level       string `json:"level"` // Good, Moderate, Poor, etc.
	PM25        int    `json:"pm2_5"` // PM2.5 concentration
	PM10        int    `json:"pm10"`  // PM10 concentration
	Ozone       int    `json:"ozone"` // Ozone concentration
	Description string `json:"description"`
}

// UVIndex UV index information
type UVIndex struct {
	Value       int    `json:"value"` // 0-11+
	Level       string `json:"level"` // Low, Moderate, High, Very High, Extreme
	Description string `json:"description"`
}

// Astronomy astronomical information
type Astronomy struct {
	Sunrise          string `json:"sunrise"`
	Sunset           string `json:"sunset"`
	Moonrise         string `json:"moonrise"`
	Moonset          string `json:"moonset"`
	MoonPhase        string `json:"moon_phase"`
	MoonIllumination int    `json:"moon_illumination"` // 0-100
}

// TimeRange time range
type TimeRange struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// HourlyForecast hourly forecast
type HourlyForecast struct {
	Time          int64   `json:"time"`
	Temperature   int     `json:"temperature"`
	Condition     string  `json:"condition"`
	Precipitation float64 `json:"precipitation"`
	Humidity      int     `json:"humidity"`
	WindSpeed     float64 `json:"wind_speed"`
}

// WeatherResponse enhanced weather response
type WeatherResponse struct {
	Location       string           `json:"location"`
	Coordinates    Coordinates      `json:"coordinates"`
	Timestamp      TimeRange        `json:"timestamp"`
	Temperature    Temperature      `json:"temperature"`
	Condition      string           `json:"condition"`
	Description    string           `json:"description"` // detailed weather description
	Humidity       int              `json:"humidity"`
	Pressure       int              `json:"pressure"`    // atmospheric pressure in hPa
	Visibility     int              `json:"visibility"`  // visibility in km
	CloudCover     int              `json:"cloud_cover"` // cloud coverage 0-100%
	DewPoint       int              `json:"dew_point"`   // dew point temperature
	Wind           Wind             `json:"wind"`
	Precipitation  *Precipitation   `json:"precipitation,omitempty"`
	AirQuality     *AirQuality      `json:"air_quality,omitempty"`
	UVIndex        UVIndex          `json:"uv_index"`
	Astronomy      Astronomy        `json:"astronomy"`
	HourlyForecast []HourlyForecast `json:"hourly_forecast,omitempty"`
	Alerts         []WeatherAlert   `json:"alerts,omitempty"` // weather alerts
	Source         string           `json:"source"`
	LastUpdated    int64            `json:"last_updated"`
}

// Coordinates geographic coordinates
type Coordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Elevation int     `json:"elevation"` // elevation in meters
}

// WeatherAlert weather alert
type WeatherAlert struct {
	Type        string `json:"type"`     // typhoon, flood, heat, cold, etc.
	Severity    string `json:"severity"` // minor, moderate, severe, extreme
	Title       string `json:"title"`
	Description string `json:"description"`
	StartTime   int64  `json:"start_time"`
	EndTime     int64  `json:"end_time"`
}

// WeatherRequest weather request
type WeatherRequest struct {
	Location string `json:"location" jsonschema:"required" jsonschema_description:"Geographic location for weather query. Can be a city name (e.g., 'Beijing', 'New York', 'Tokyo'), city with country (e.g., 'Paris, France'), or specific address. Supports both English and local language names. For more accurate results, include country or region information."`

	Date string `json:"date" jsonschema_description:"Target date for weather forecast in YYYY-MM-DD format (e.g., '2024-01-15'). If not provided or empty string, defaults to current date. For historical weather simulation, use past dates. For future forecast, use dates up to 14 days ahead. Must be a valid date string."`

	IncludeHourly bool `json:"include_hourly" jsonschema_description:"Whether to include 24-hour detailed hourly forecast in the response. When set to true, returns hour-by-hour weather conditions including temperature, precipitation, humidity, and wind speed for each hour of the day. When false, only returns daily summary. Default is false. Recommended for detailed day planning."`

	IncludeAirQuality bool `json:"include_air_quality" jsonschema_description:"Whether to include air quality information in the response. When set to true, returns detailed air quality data including AQI (Air Quality Index), PM2.5, PM10, ozone levels, and health recommendations. Particularly useful for urban areas and users with respiratory sensitivities. When false, air quality data is omitted. Default is false."`
}

// ClimateZone climate zone type
type ClimateZone int

const (
	Tropical ClimateZone = iota
	Subtropical
	Temperate
	Continental
	Polar
	Desert
	Mediterranean
	Oceanic
	Alpine
)

// SeasonalPattern seasonal pattern
type SeasonalPattern struct {
	RainySeasonStart int // rainy season start month
	RainySeasonEnd   int // rainy season end month
	DrySeason        bool
	MonsoonInfluence bool
}

// GenerateFakeWeatherResponse generates enhanced fake weather response
func GenerateFakeWeatherResponse(req *WeatherRequest) (*WeatherResponse, error) {
	// Parse date
	var targetDate time.Time
	var err error

	if req.Date == "" {
		targetDate = time.Now()
	} else {
		targetDate, err = time.Parse("2006-01-02", req.Date)
		if err != nil {
			return nil, fmt.Errorf("invalid date format: %v", err)
		}
	}

	// Create deterministic random number generator
	seed := uint64(targetDate.Unix() + int64(len(req.Location)))
	for _, c := range req.Location {
		seed = seed*31 + uint64(c)
	}
	rng := rand.New(rand.NewPCG(seed, seed))

	// Identify climate zone and geographic information
	zone := identifyClimateZone(req.Location)
	coords := generateCoordinates(req.Location, rng)
	seasonal := getSeasonalPattern(zone)

	// Synthesize base temperature
	month := int(targetDate.Month())
	hour := targetDate.Hour()
	baseTemp := getBaseTemperature(month, zone, coords.Latitude)

	// Consider daily temperature variation
	tempVariation := getDailyTemperatureVariation(hour, zone)
	currentTemp := baseTemp + tempVariation + rng.IntN(5) - 2

	// Consider elevation effect (approximately 0.6°C decrease per 100m elevation)
	elevationEffect := -int(float64(coords.Elevation) * 0.006)
	currentTemp += elevationEffect

	// Synthesize temperature range
	minTemp := currentTemp - 3 - rng.IntN(5)
	maxTemp := currentTemp + 3 + rng.IntN(5)

	// Synthesize weather conditions
	conditions := getReasonableConditions(currentTemp, month, zone, seasonal)
	condition := conditions[rng.IntN(len(conditions))]

	// Synthesize wind information
	wind := generateWind(condition, zone, coords, rng)

	// Synthesize humidity
	humidity := generateHumidity(condition, zone, month, seasonal, rng)

	// Calculate feels like temperature
	feelsLike := calculateFeelsLike(currentTemp, humidity, wind.Speed)

	// Synthesize pressure (standard pressure 1013 hPa, considering elevation and weather)
	pressure := generatePressure(coords.Elevation, condition, rng)

	// Synthesize visibility
	visibility := generateVisibility(condition, humidity, rng)

	// Synthesize cloud cover
	cloudCover := generateCloudCover(condition, rng)

	// Calculate dew point temperature
	dewPoint := calculateDewPoint(currentTemp, humidity)

	// Synthesize precipitation information
	var precipitation *Precipitation
	if needsPrecipitation(condition) {
		precipitation = generatePrecipitation(condition, currentTemp, month, seasonal, rng)
	}

	// Synthesize air quality
	var airQuality *AirQuality
	if req.IncludeAirQuality {
		airQuality = generateAirQuality(req.Location, condition, zone, rng)
	}

	// Synthesize UV index
	uvIndex := generateUVIndex(month, coords.Latitude, condition, cloudCover, rng)

	// Synthesize astronomical information
	astronomy := generateAstronomy(targetDate, coords, rng)

	// Synthesize hourly forecast
	var hourlyForecast []HourlyForecast
	if req.IncludeHourly {
		hourlyForecast = generateHourlyForecast(targetDate, baseTemp, condition, zone, rng)
	}

	// Synthesize weather alerts
	alerts := generateWeatherAlerts(condition, currentTemp, wind.Speed, zone, targetDate, rng)

	// Synthesize detailed description
	description := generateWeatherDescription(condition, currentTemp, wind, humidity, precipitation)

	// Time range
	startOfDay := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, targetDate.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	return &WeatherResponse{
		Location:    req.Location,
		Coordinates: coords,
		Timestamp: TimeRange{
			Start: startOfDay.Unix(),
			End:   endOfDay.Unix(),
		},
		Temperature: Temperature{
			Value:     currentTemp,
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
		Source:         "WeatherAPI Enhanced v2.0",
		LastUpdated:    time.Now().Unix(),
	}, nil
}

// generateCoordinates generates geographic coordinates
func generateCoordinates(location string, rng *rand.Rand) Coordinates {
	// Can implement more complex city coordinate mapping here
	// Simplified version: generate pseudo-random coordinates based on location string

	locationLower := strings.ToLower(location)

	// Predefined coordinates for major cities
	cityCoords := map[string]Coordinates{
		"beijing":   {39.9042, 116.4074, 43},
		"shanghai":  {31.2304, 121.4737, 4},
		"guangzhou": {23.1291, 113.2644, 21},
		"shenzhen":  {22.5431, 114.0579, 19},
		"tokyo":     {35.6762, 139.6503, 40},
		"singapore": {1.3521, 103.8198, 15},
		"new york":  {40.7128, -74.0060, 10},
		"london":    {51.5074, -0.1278, 11},
		"paris":     {48.8566, 2.3522, 35},
		"sydney":    {-33.8688, 151.2093, 58},
		"dubai":     {25.2048, 55.2708, 5},
		"moscow":    {55.7558, 37.6173, 156},
	}

	for city, coords := range cityCoords {
		if strings.Contains(locationLower, city) {
			return coords
		}
	}

	// If not found, generate pseudo-random coordinates
	latSeed := 0.0
	lonSeed := 0.0
	for _, c := range location {
		latSeed += float64(c)
		lonSeed += float64(c) * 1.5
	}

	lat := math.Mod(latSeed, 180) - 90
	lon := math.Mod(lonSeed, 360) - 180
	elevation := rng.IntN(500)

	return Coordinates{
		Latitude:  math.Round(lat*10000) / 10000,
		Longitude: math.Round(lon*10000) / 10000,
		Elevation: elevation,
	}
}

// getSeasonalPattern gets seasonal pattern
func getSeasonalPattern(zone ClimateZone) SeasonalPattern {
	switch zone {
	case Tropical:
		return SeasonalPattern{
			RainySeasonStart: 5,
			RainySeasonEnd:   10,
			MonsoonInfluence: true,
		}
	case Subtropical:
		return SeasonalPattern{
			RainySeasonStart: 4,
			RainySeasonEnd:   9,
			MonsoonInfluence: true,
		}
	case Mediterranean:
		return SeasonalPattern{
			RainySeasonStart: 11,
			RainySeasonEnd:   3,
			DrySeason:        true,
		}
	case Desert:
		return SeasonalPattern{
			DrySeason: true,
		}
	default:
		return SeasonalPattern{}
	}
}

// getDailyTemperatureVariation gets daily temperature variation
func getDailyTemperatureVariation(hour int, zone ClimateZone) int {
	amplitude := 5 // default amplitude

	switch zone {
	case Desert:
		amplitude = 12 // large day-night temperature difference in desert
	case Oceanic:
		amplitude = 3 // small temperature difference in oceanic climate
	case Tropical:
		amplitude = 4
	}

	// Use sine function to simulate temperature variation, hottest at 14:00, coldest at 6:00
	radians := float64(hour-6) * math.Pi / 12
	variation := amplitude * int(math.Sin(radians))

	return variation
}

// calculateFeelsLike calculates feels like temperature
func calculateFeelsLike(temp int, humidity int, windSpeed float64) int {
	// Simplified feels like temperature calculation
	feelsLike := float64(temp)

	// Wind chill effect (when temperature is below 10 degrees)
	if temp < 10 && windSpeed > 4.8 {
		windChill := 13.12 + 0.6215*float64(temp) - 11.37*math.Pow(windSpeed, 0.16) +
			0.3965*float64(temp)*math.Pow(windSpeed, 0.16)
		feelsLike = windChill
	}

	// Heat index (when temperature is above 27 degrees and humidity is high)
	if temp > 27 && humidity > 40 {
		t := float64(temp)
		rh := float64(humidity)
		heatIndex := -8.78469475556 + 1.61139411*t + 2.33854883889*rh -
			0.14611605*t*rh - 0.012308094*t*t - 0.0164248277778*rh*rh +
			0.002211732*t*t*rh + 0.00072546*t*rh*rh - 0.000003582*t*t*rh*rh
		feelsLike = heatIndex
	}

	return int(math.Round(feelsLike))
}

// generateWind generates wind information
func generateWind(condition string, zone ClimateZone, coords Coordinates, rng *rand.Rand) Wind {
	baseSpeed := 5.0 + rng.Float64()*15.0

	// Adjust based on weather conditions
	switch condition {
	case "Stormy", "Blizzard":
		baseSpeed += rng.Float64() * 30.0
	case "Rainy", "Snowy":
		baseSpeed += rng.Float64() * 15.0
	case "Sunny", "Clear":
		baseSpeed *= 0.6
	}

	// Adjust based on climate zone
	switch zone {
	case Desert:
		baseSpeed += rng.Float64() * 10.0
	case Oceanic:
		baseSpeed += rng.Float64() * 8.0
	case Alpine:
		baseSpeed += rng.Float64() * 12.0
	}

	// Elevation effect (higher elevation results in higher wind speed)
	baseSpeed += float64(coords.Elevation) * 0.01

	degree := rng.IntN(360)
	direction := degreeToDirection(degree)

	// Gust speed is 20-50% higher than average wind speed
	gust := baseSpeed * (1.2 + rng.Float64()*0.3)

	return Wind{
		Speed:     math.Round(baseSpeed*10) / 10,
		Unit:      "km/h",
		Direction: direction,
		Degree:    degree,
		Gust:      math.Round(gust*10) / 10,
	}
}

// degreeToDirection converts degree to direction
func degreeToDirection(degree int) string {
	directions := []string{
		"North", "North-North-East", "North-East", "East-North-East",
		"East", "East-South-East", "South-East", "South-South-East",
		"South", "South-South-West", "South-West", "West-South-West",
		"West", "West-North-West", "North-West", "North-North-West",
	}

	index := int(math.Round(float64(degree)/22.5)) % 16
	return directions[index]
}

// generatePressure generates atmospheric pressure
func generatePressure(elevation int, condition string, rng *rand.Rand) int {
	// Standard pressure 1013 hPa, decreases by approximately 1 hPa per 8m elevation
	basePressure := 1013 - elevation/8

	// Adjust based on weather conditions
	adjustment := 0
	switch condition {
	case "Stormy", "Rainy":
		adjustment = -10 - rng.IntN(15)
	case "Sunny", "Clear":
		adjustment = 5 + rng.IntN(10)
	case "Cloudy", "Partly Cloudy":
		adjustment = rng.IntN(10) - 5
	}

	return basePressure + adjustment
}

// generateVisibility generates visibility
func generateVisibility(condition string, humidity int, rng *rand.Rand) int {
	baseVisibility := 10 // km

	switch condition {
	case "Foggy", "Mist":
		baseVisibility = rng.IntN(2) + 1
	case "Rainy", "Snowy":
		baseVisibility = 3 + rng.IntN(5)
	case "Stormy", "Blizzard":
		baseVisibility = 1 + rng.IntN(3)
	case "Dusty", "Hazy":
		baseVisibility = 2 + rng.IntN(6)
	case "Cloudy":
		baseVisibility = 8 + rng.IntN(7)
	case "Sunny", "Clear":
		baseVisibility = 15 + rng.IntN(35)
	default:
		baseVisibility = 10 + rng.IntN(10)
	}

	// High humidity reduces visibility
	if humidity > 85 {
		baseVisibility = int(float64(baseVisibility) * 0.7)
	}

	return max(1, baseVisibility)
}

// generateCloudCover generates cloud coverage
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
	default:
		return 40 + rng.IntN(40)
	}
}

// calculateDewPoint calculates dew point temperature
func calculateDewPoint(temp int, humidity int) int {
	// Magnus formula for dew point calculation
	a := 17.27
	b := 237.7

	t := float64(temp)
	rh := float64(humidity) / 100.0

	alpha := (a*t)/(b+t) + math.Log(rh)
	dewPoint := (b * alpha) / (a - alpha)

	return int(math.Round(dewPoint))
}

// needsPrecipitation determines if precipitation information is needed
func needsPrecipitation(condition string) bool {
	precipConditions := []string{"Rainy", "Snowy", "Stormy", "Blizzard", "Sleet", "Drizzle"}
	for _, c := range precipConditions {
		if condition == c {
			return true
		}
	}
	return false
}

// generatePrecipitation generates precipitation information
func generatePrecipitation(condition string, temp int, month int, seasonal SeasonalPattern, rng *rand.Rand) *Precipitation {
	precip := &Precipitation{}

	// Determine precipitation type
	if temp < 0 {
		precip.Type = "snow"
	} else if temp < 3 {
		if rng.Float64() < 0.3 {
			precip.Type = "sleet"
		} else {
			precip.Type = "snow"
		}
	} else {
		precip.Type = "rain"
	}

	// Set precipitation probability
	switch condition {
	case "Stormy", "Blizzard":
		precip.Probability = 85 + rng.IntN(15)
	case "Rainy", "Snowy":
		precip.Probability = 60 + rng.IntN(30)
	case "Drizzle":
		precip.Probability = 40 + rng.IntN(30)
	default:
		precip.Probability = 30 + rng.IntN(40)
	}

	// Increase probability during rainy season
	if seasonal.MonsoonInfluence && month >= seasonal.RainySeasonStart && month <= seasonal.RainySeasonEnd {
		precip.Probability = min(100, precip.Probability+15)
	}

	// Synthesize precipitation amount
	switch condition {
	case "Stormy":
		precip.Amount = 20.0 + rng.Float64()*40.0
		precip.Intensity = "heavy"
	case "Rainy":
		precip.Amount = 5.0 + rng.Float64()*20.0
		if precip.Amount > 15 {
			precip.Intensity = "moderate"
		} else {
			precip.Intensity = "light"
		}
	case "Drizzle":
		precip.Amount = 0.5 + rng.Float64()*3.0
		precip.Intensity = "light"
	case "Snowy", "Blizzard":
		// Snow precipitation is typically 1/10 of rain
		precip.Amount = 1.0 + rng.Float64()*10.0
		if condition == "Blizzard" {
			precip.Intensity = "heavy"
		} else {
			precip.Intensity = "moderate"
		}
	default:
		precip.Amount = rng.Float64() * 5.0
		precip.Intensity = "light"
	}

	precip.Amount = math.Round(precip.Amount*10) / 10

	return precip
}

// generateAirQuality generates air quality information
func generateAirQuality(location string, condition string, zone ClimateZone, rng *rand.Rand) *AirQuality {
	aq := &AirQuality{}

	// Base AQI
	baseAQI := 50

	// Big cities typically have more severe pollution
	bigCities := []string{"beijing", "delhi", "shanghai", "mumbai", "mexico city", "cairo"}
	locationLower := strings.ToLower(location)
	for _, city := range bigCities {
		if strings.Contains(locationLower, city) {
			baseAQI = 80 + rng.IntN(40)
			break
		}
	}

	// Weather conditions impact
	switch condition {
	case "Foggy", "Hazy":
		baseAQI += 40 + rng.IntN(30)
	case "Rainy", "Stormy":
		baseAQI -= 20 + rng.IntN(20) // Rain purifies air
	case "Windy":
		baseAQI -= 10 + rng.IntN(15) // Wind disperses pollution
	}

	// Climate zone impact
	if zone == Desert {
		baseAQI += 10 + rng.IntN(20) // Sand and dust
	}

	aq.AQI = max(0, min(500, baseAQI))

	// Set level
	switch {
	case aq.AQI <= 50:
		aq.Level = "Good"
		aq.Description = "Air quality is satisfactory, and air pollution poses little or no risk."
	case aq.AQI <= 100:
		aq.Level = "Moderate"
		aq.Description = "Air quality is acceptable. However, there may be a risk for some people."
	case aq.AQI <= 150:
		aq.Level = "Unhealthy for Sensitive Groups"
		aq.Description = "Members of sensitive groups may experience health effects."
	case aq.AQI <= 200:
		aq.Level = "Unhealthy"
		aq.Description = "Some members of the general public may experience health effects."
	case aq.AQI <= 300:
		aq.Level = "Very Unhealthy"
		aq.Description = "Health alert: The risk of health effects is increased for everyone."
	default:
		aq.Level = "Hazardous"
		aq.Description = "Health warning of emergency conditions: everyone is more likely to be affected."
	}

	// Synthesize pollutant concentrations
	aq.PM25 = int(float64(aq.AQI) * 0.5 * (1 + rng.Float64()*0.4))
	aq.PM10 = int(float64(aq.PM25) * 1.5 * (1 + rng.Float64()*0.3))
	aq.Ozone = 20 + rng.IntN(80)

	return aq
}

// generateUVIndex generates UV index
func generateUVIndex(month int, latitude float64, condition string, cloudCover int, rng *rand.Rand) UVIndex {
	uv := UVIndex{}

	// Base UV value based on latitude and month
	// UV is strongest near equator, weakest at poles
	absLat := math.Abs(latitude)
	latitudeFactor := 1.0 - absLat/90.0

	// UV is stronger in summer
	var seasonFactor float64
	if month >= 5 && month <= 8 {
		seasonFactor = 1.2
	} else if month >= 11 || month <= 2 {
		seasonFactor = 0.6
	} else {
		seasonFactor = 0.9
	}

	baseUV := int(11.0 * latitudeFactor * seasonFactor)

	// Cloud cover reduces UV
	cloudReduction := int(float64(cloudCover) * 0.08)
	baseUV -= cloudReduction

	// Specific weather conditions
	switch condition {
	case "Sunny", "Clear":
		baseUV += 1 + rng.IntN(2)
	case "Cloudy", "Overcast":
		baseUV -= 2 + rng.IntN(2)
	case "Rainy", "Stormy":
		baseUV -= 4 + rng.IntN(3)
	}

	uv.Value = max(0, min(11, baseUV))

	// Set level
	switch {
	case uv.Value <= 2:
		uv.Level = "Low"
		uv.Description = "No protection required. You can safely stay outside."
	case uv.Value <= 5:
		uv.Level = "Moderate"
		uv.Description = "Seek shade during midday hours. Wear sunscreen and a hat."
	case uv.Value <= 7:
		uv.Level = "High"
		uv.Description = "Protection essential. Seek shade during midday hours."
	case uv.Value <= 10:
		uv.Level = "Very High"
		uv.Description = "Extra protection needed. Avoid sun exposure during midday."
	default:
		uv.Level = "Extreme"
		uv.Description = "Take all precautions. Unprotected skin will burn quickly."
	}

	return uv
}

// generateAstronomy generates astronomical information
func generateAstronomy(date time.Time, coords Coordinates, rng *rand.Rand) Astronomy {
	// Using simplified calculations here, actual applications can use more precise astronomical algorithms

	// Estimate sunrise and sunset times based on latitude
	dayOfYear := date.YearDay()

	// Simplified daylight duration calculation (without precise astronomical calculations)
	declination := 23.45 * math.Sin(2*math.Pi*(float64(dayOfYear)-81)/365)
	lat := coords.Latitude

	hourAngle := math.Acos(-math.Tan(lat*math.Pi/180) * math.Tan(declination*math.Pi/180))
	daylightHours := 2 * hourAngle * 12 / math.Pi

	// Sunrise time (simplified)
	sunriseHour := 12 - daylightHours/2
	sunriseMinute := int((sunriseHour - math.Floor(sunriseHour)) * 60)
	sunrise := fmt.Sprintf("%02d:%02d", int(sunriseHour), sunriseMinute)

	// Sunset time
	sunsetHour := 12 + daylightHours/2
	sunsetMinute := int((sunsetHour - math.Floor(sunsetHour)) * 60)
	sunset := fmt.Sprintf("%02d:%02d", int(sunsetHour), sunsetMinute)

	// Moon phase (simplified calculation)
	moonPhases := []string{"New Moon", "Waxing Crescent", "First Quarter", "Waxing Gibbous",
		"Full Moon", "Waning Gibbous", "Last Quarter", "Waning Crescent"}
	phaseIndex := (dayOfYear % 29) * 8 / 29
	moonPhase := moonPhases[phaseIndex]

	moonIllumination := int(math.Abs(math.Sin(float64(dayOfYear)*2*math.Pi/29.5)) * 100)

	// Moonrise and moonset times (offset relative to sunrise and sunset)
	moonriseOffset := rng.IntN(120) - 60
	moonsetOffset := rng.IntN(120) - 60

	moonriseHour := int(sunriseHour) + moonriseOffset/60
	moonriseMinute := (sunriseMinute + moonriseOffset) % 60
	moonrise := fmt.Sprintf("%02d:%02d", moonriseHour%24, moonriseMinute)

	moonsetHour := int(sunsetHour) + moonsetOffset/60
	moonsetMinute := (sunsetMinute + moonsetOffset) % 60
	moonset := fmt.Sprintf("%02d:%02d", moonsetHour%24, moonsetMinute)

	return Astronomy{
		Sunrise:          sunrise,
		Sunset:           sunset,
		Moonrise:         moonrise,
		Moonset:          moonset,
		MoonPhase:        moonPhase,
		MoonIllumination: moonIllumination,
	}
}

// generateHourlyForecast generates hourly forecast
func generateHourlyForecast(date time.Time, baseTemp int, condition string, zone ClimateZone, rng *rand.Rand) []HourlyForecast {
	forecasts := make([]HourlyForecast, 24)

	for i := 0; i < 24; i++ {
		hour := time.Date(date.Year(), date.Month(), date.Day(), i, 0, 0, 0, date.Location())

		// Temperature variation
		tempVariation := getDailyTemperatureVariation(i, zone)
		hourTemp := baseTemp + tempVariation + rng.IntN(3) - 1

		// Weather conditions may change during the day
		hourCondition := condition
		if rng.Float64() < 0.2 {
			// 20% chance of weather change
			conditions := getReasonableConditions(hourTemp, int(date.Month()), zone, SeasonalPattern{})
			hourCondition = conditions[rng.IntN(len(conditions))]
		}

		// Precipitation amount
		precipitation := 0.0
		if needsPrecipitation(hourCondition) {
			precipitation = rng.Float64() * 5.0
		}

		// Humidity is higher at night
		humidity := 50 + rng.IntN(30)
		if i >= 22 || i <= 6 {
			humidity += 10
		}
		humidity = min(100, humidity)

		// Wind speed
		windSpeed := 5.0 + rng.Float64()*15.0

		forecasts[i] = HourlyForecast{
			Time:          hour.Unix(),
			Temperature:   hourTemp,
			Condition:     hourCondition,
			Precipitation: math.Round(precipitation*10) / 10,
			Humidity:      humidity,
			WindSpeed:     math.Round(windSpeed*10) / 10,
		}
	}

	return forecasts
}

// generateWeatherAlerts generates weather alerts
func generateWeatherAlerts(condition string, temp int, windSpeed float64, zone ClimateZone, date time.Time, rng *rand.Rand) []WeatherAlert {
	var alerts []WeatherAlert

	// High temperature alert
	if temp >= 35 {
		severity := "moderate"
		if temp >= 40 {
			severity = "severe"
		}
		alerts = append(alerts, WeatherAlert{
			Type:        "heat",
			Severity:    severity,
			Title:       "High Temperature Warning",
			Description: fmt.Sprintf("Temperature is expected to reach %d°C. Stay hydrated and avoid prolonged sun exposure.", temp),
			StartTime:   date.Unix(),
			EndTime:     date.Add(24 * time.Hour).Unix(),
		})
	}

	// Low temperature alert
	if temp <= -10 {
		severity := "moderate"
		if temp <= -20 {
			severity = "severe"
		}
		alerts = append(alerts, WeatherAlert{
			Type:        "cold",
			Severity:    severity,
			Title:       "Extreme Cold Warning",
			Description: fmt.Sprintf("Temperature is expected to drop to %d°C. Dress warmly and limit outdoor exposure.", temp),
			StartTime:   date.Unix(),
			EndTime:     date.Add(24 * time.Hour).Unix(),
		})
	}

	// Strong wind alert
	if windSpeed >= 50 {
		severity := "moderate"
		if windSpeed >= 70 {
			severity = "severe"
		}
		alerts = append(alerts, WeatherAlert{
			Type:        "wind",
			Severity:    severity,
			Title:       "High Wind Warning",
			Description: fmt.Sprintf("Wind speeds may reach %.1f km/h. Secure loose objects and avoid outdoor activities.", windSpeed),
			StartTime:   date.Unix(),
			EndTime:     date.Add(12 * time.Hour).Unix(),
		})
	}

	// Heavy rain alert
	if condition == "Stormy" {
		alerts = append(alerts, WeatherAlert{
			Type:        "storm",
			Severity:    "severe",
			Title:       "Severe Storm Warning",
			Description: "Severe thunderstorms expected. Stay indoors and avoid travel if possible.",
			StartTime:   date.Unix(),
			EndTime:     date.Add(6 * time.Hour).Unix(),
		})
	}

	// Blizzard alert
	if condition == "Blizzard" {
		alerts = append(alerts, WeatherAlert{
			Type:        "snow",
			Severity:    "severe",
			Title:       "Blizzard Warning",
			Description: "Blizzard conditions expected with heavy snow and strong winds. Travel is strongly discouraged.",
			StartTime:   date.Unix(),
			EndTime:     date.Add(12 * time.Hour).Unix(),
		})
	}

	// Typhoon alert (tropical and subtropical regions, specific months)
	month := int(date.Month())
	if (zone == Tropical || zone == Subtropical) && month >= 6 && month <= 10 && rng.Float64() < 0.05 {
		alerts = append(alerts, WeatherAlert{
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

// generateWeatherDescription generates weather description
func generateWeatherDescription(condition string, temp int, wind Wind, humidity int, precipitation *Precipitation) string {
	desc := ""

	// Base weather description
	switch condition {
	case "Sunny":
		desc = "Clear skies with abundant sunshine throughout the day."
	case "Partly Cloudy":
		desc = "Mix of sun and clouds with pleasant weather conditions."
	case "Cloudy":
		desc = "Overcast skies with cloud cover throughout the day."
	case "Rainy":
		desc = "Rainy conditions expected."
		if precipitation != nil {
			desc += fmt.Sprintf(" Rainfall amount: %.1f mm. %s intensity.", precipitation.Amount, precipitation.Intensity)
		}
	case "Stormy":
		desc = "Severe thunderstorms with heavy rain and strong winds. Lightning activity expected."
	case "Snowy":
		desc = "Snow is expected."
		if precipitation != nil {
			desc += fmt.Sprintf(" Snowfall amount: %.1f mm. %s intensity.", precipitation.Amount, precipitation.Intensity)
		}
	case "Blizzard":
		desc = "Blizzard conditions with heavy snow and very strong winds. Visibility severely reduced."
	case "Foggy":
		desc = "Dense fog reducing visibility significantly. Drive with caution."
	case "Hot":
		desc = "Hot and sunny conditions. Take precautions against heat."
	default:
		desc = fmt.Sprintf("%s weather conditions expected.", condition)
	}

	// Add temperature description
	if temp < 0 {
		desc += " Freezing temperatures."
	} else if temp > 30 {
		desc += " High temperatures."
	}

	// Add wind description
	if wind.Speed > 30 {
		desc += fmt.Sprintf(" Strong winds from the %s at %.1f km/h.", strings.ToLower(wind.Direction), wind.Speed)
	} else if wind.Speed > 15 {
		desc += fmt.Sprintf(" Moderate winds from the %s.", strings.ToLower(wind.Direction))
	}

	// Add humidity description
	if humidity > 80 {
		desc += " High humidity making it feel muggy."
	} else if humidity < 30 {
		desc += " Low humidity with dry air."
	}

	return desc
}

// identifyClimateZone identifies climate zone (enhanced version)
func identifyClimateZone(location string) ClimateZone {
	loc := strings.ToLower(location)

	// Tropical
	tropicalKeywords := []string{
		"singapore", "jakarta", "manila", "bangkok", "kuala lumpur",
		"mumbai", "chennai", "ho chi minh", "colombo", "hanoi",
	}
	if containsAny(loc, tropicalKeywords...) {
		return Tropical
	}

	// Subtropical
	subtropicalKeywords := []string{
		"hong kong", "taiwan", "guangzhou", "shenzhen", "miami",
		"sydney", "brisbane", "rio de janeiro", "shanghai", "tokyo",
	}
	if containsAny(loc, subtropicalKeywords...) {
		return Subtropical
	}

	// Mediterranean
	mediterraneanKeywords := []string{
		"rome", "athens", "barcelona", "madrid", "lisbon",
		"los angeles", "san francisco", "perth", "cape town",
	}
	if containsAny(loc, mediterraneanKeywords...) {
		return Mediterranean
	}

	// Desert
	desertKeywords := []string{
		"dubai", "riyadh", "cairo", "phoenix", "las vegas",
		"alice springs", "sahara", "gobi", "doha",
	}
	if containsAny(loc, desertKeywords...) {
		return Desert
	}

	// Polar
	polarKeywords := []string{
		"alaska", "reykjavik", "murmansk", "antarctica",
		"yellowknife", "nuuk", "svalbard", "barrow",
	}
	if containsAny(loc, polarKeywords...) {
		return Polar
	}

	// Continental
	continentalKeywords := []string{
		"moscow", "beijing", "harbin", "chicago", "denver",
		"ulaanbaatar", "almaty", "winnipeg", "montreal",
	}
	if containsAny(loc, continentalKeywords...) {
		return Continental
	}

	// Oceanic
	oceanicKeywords := []string{
		"london", "paris", "dublin", "amsterdam", "seattle",
		"vancouver", "wellington", "copenhagen",
	}
	if containsAny(loc, oceanicKeywords...) {
		return Oceanic
	}

	// Alpine
	alpineKeywords := []string{
		"geneva", "innsbruck", "aspen", "kathmandu", "lhasa",
		"cusco", "la paz", "quito",
	}
	if containsAny(loc, alpineKeywords...) {
		return Alpine
	}

	return Temperate
}

// getBaseTemperature gets base temperature (enhanced version)
func getBaseTemperature(month int, zone ClimateZone, latitude float64) int {
	// Determine northern or southern hemisphere
	isNorthern := latitude >= 0

	// If southern hemisphere, offset month by 6 months
	if !isNorthern {
		month = (month + 6) % 12
		if month == 0 {
			month = 12
		}
	}

	switch zone {
	case Tropical:
		return 28 + (month%3 - 1)

	case Subtropical:
		temps := map[int]int{
			1: 10, 2: 12, 3: 16, 4: 22, 5: 26, 6: 30,
			7: 32, 8: 31, 9: 28, 10: 22, 11: 16, 12: 11,
		}
		return temps[month]

	case Mediterranean:
		temps := map[int]int{
			1: 12, 2: 13, 3: 15, 4: 18, 5: 22, 6: 27,
			7: 30, 8: 30, 9: 26, 10: 21, 11: 16, 12: 13,
		}
		return temps[month]

	case Desert:
		temps := map[int]int{
			1: 15, 2: 18, 3: 22, 4: 28, 5: 35, 6: 40,
			7: 42, 8: 41, 9: 37, 10: 30, 11: 22, 12: 16,
		}
		return temps[month]

	case Continental:
		temps := map[int]int{
			1: -5, 2: -2, 3: 5, 4: 14, 5: 21, 6: 26,
			7: 28, 8: 26, 9: 20, 10: 12, 11: 3, 12: -3,
		}
		return temps[month]

	case Polar:
		temps := map[int]int{
			1: -25, 2: -22, 3: -15, 4: -8, 5: -2, 6: 3,
			7: 5, 8: 4, 9: -1, 10: -10, 11: -18, 12: -23,
		}
		return temps[month]

	case Oceanic:
		temps := map[int]int{
			1: 7, 2: 8, 3: 10, 4: 13, 5: 16, 6: 19,
			7: 21, 8: 21, 9: 18, 10: 14, 11: 10, 12: 8,
		}
		return temps[month]

	case Alpine:
		temps := map[int]int{
			1: -5, 2: -3, 3: 2, 4: 8, 5: 13, 6: 17,
			7: 19, 8: 18, 9: 14, 10: 9, 11: 2, 12: -3,
		}
		return temps[month]

	default: // Temperate
		temps := map[int]int{
			1: 5, 2: 7, 3: 12, 4: 18, 5: 23, 6: 28,
			7: 30, 8: 29, 9: 24, 10: 18, 11: 12, 12: 7,
		}
		return temps[month]
	}
}

// getReasonableConditions gets reasonable weather conditions (enhanced version)
func getReasonableConditions(temp int, month int, zone ClimateZone, seasonal SeasonalPattern) []string {
	isSummer := month >= 6 && month <= 8
	isWinter := month >= 12 || month <= 2
	isRainySeason := seasonal.MonsoonInfluence && month >= seasonal.RainySeasonStart && month <= seasonal.RainySeasonEnd

	switch zone {
	case Tropical:
		if isRainySeason {
			return []string{"Rainy", "Stormy", "Partly Cloudy", "Humid", "Drizzle"}
		}
		return []string{"Partly Cloudy", "Humid", "Sunny", "Rainy"}

	case Desert:
		if temp > 38 {
			return []string{"Sunny", "Hot", "Clear", "Dusty", "Hazy"}
		}
		return []string{"Sunny", "Clear", "Partly Cloudy", "Dusty"}

	case Mediterranean:
		if isSummer {
			return []string{"Sunny", "Clear", "Hot", "Partly Cloudy"}
		}
		return []string{"Rainy", "Cloudy", "Partly Cloudy", "Clear", "Drizzle"}

	case Polar:
		if temp < -15 {
			return []string{"Snowy", "Blizzard", "Cloudy", "Freezing", "Clear"}
		}
		return []string{"Snowy", "Cloudy", "Clear", "Cold", "Overcast"}

	case Continental:
		if temp < -5 {
			return []string{"Snowy", "Cloudy", "Clear", "Cold", "Blizzard"}
		}
		if temp > 28 && isSummer {
			return []string{"Sunny", "Hot", "Stormy", "Partly Cloudy", "Clear"}
		}
		return []string{"Sunny", "Partly Cloudy", "Cloudy", "Clear", "Rainy"}

	case Oceanic:
		if isWinter {
			return []string{"Rainy", "Cloudy", "Drizzle", "Overcast", "Foggy"}
		}
		return []string{"Partly Cloudy", "Cloudy", "Sunny", "Rainy", "Clear"}

	case Alpine:
		if temp < 5 {
			return []string{"Snowy", "Cloudy", "Clear", "Cold", "Windy"}
		}
		return []string{"Partly Cloudy", "Sunny", "Clear", "Cloudy", "Rainy"}

	default: // Temperate
		switch {
		case temp < 0:
			return []string{"Snowy", "Cloudy", "Clear", "Cold", "Freezing"}
		case temp < 10:
			return []string{"Cloudy", "Clear", "Rainy", "Foggy", "Drizzle"}
		case temp < 25:
			return []string{"Sunny", "Partly Cloudy", "Cloudy", "Clear", "Mild"}
		default:
			if isSummer {
				return []string{"Sunny", "Partly Cloudy", "Rainy", "Stormy", "Hot"}
			}
			return []string{"Sunny", "Hot", "Partly Cloudy", "Clear"}
		}
	}
}

// generateHumidity generates humidity
func generateHumidity(condition string, zone ClimateZone, month int, seasonal SeasonalPattern, rng *rand.Rand) int {
	baseHumidity := 50

	switch zone {
	case Tropical:
		baseHumidity = 75
		if seasonal.MonsoonInfluence && month >= seasonal.RainySeasonStart && month <= seasonal.RainySeasonEnd {
			baseHumidity = 85
		}
	case Desert:
		baseHumidity = 20
	case Mediterranean:
		if month >= 6 && month <= 9 {
			baseHumidity = 45 // Dry summer
		} else {
			baseHumidity = 65
		}
	case Polar:
		baseHumidity = 70
	case Oceanic:
		baseHumidity = 75
	case Alpine:
		baseHumidity = 60
	case Continental:
		baseHumidity = 55
	}

	switch condition {
	case "Rainy", "Stormy", "Foggy", "Humid", "Drizzle":
		return min(baseHumidity+20+rng.IntN(20), 100)
	case "Snowy", "Blizzard":
		return min(baseHumidity+15+rng.IntN(15), 95)
	case "Cloudy", "Partly Cloudy", "Overcast":
		return baseHumidity + rng.IntN(15)
	case "Sunny", "Clear", "Hot":
		return max(baseHumidity-20+rng.IntN(20), 10)
	case "Dusty", "Hazy":
		return max(baseHumidity-30+rng.IntN(15), 5)
	default:
		return baseHumidity + rng.IntN(20) - 10
	}
}

// containsAny checks if string contains any of the keywords
func containsAny(s string, keywords ...string) bool {
	for _, keyword := range keywords {
		if strings.Contains(strings.ToLower(s), keyword) {
			return true
		}
	}
	return false
}
