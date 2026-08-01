package fakeweather

// Condition is the closed weather-condition vocabulary shared by generation
// and the public response.
type Condition string

const (
	ConditionBlizzard     Condition = "Blizzard"
	ConditionClear        Condition = "Clear"
	ConditionCloudy       Condition = "Cloudy"
	ConditionCold         Condition = "Cold"
	ConditionDrizzle      Condition = "Drizzle"
	ConditionDusty        Condition = "Dusty"
	ConditionFoggy        Condition = "Foggy"
	ConditionFreezing     Condition = "Freezing"
	ConditionHazy         Condition = "Hazy"
	ConditionHot          Condition = "Hot"
	ConditionHumid        Condition = "Humid"
	ConditionMild         Condition = "Mild"
	ConditionOvercast     Condition = "Overcast"
	ConditionPartlyCloudy Condition = "Partly Cloudy"
	ConditionRainy        Condition = "Rainy"
	ConditionSnowy        Condition = "Snowy"
	ConditionStormy       Condition = "Stormy"
	ConditionSunny        Condition = "Sunny"
	ConditionWindy        Condition = "Windy"
)

// PrecipitationType is the synthesized precipitation phase.
type PrecipitationType string

const (
	PrecipitationRain  PrecipitationType = "rain"
	PrecipitationSleet PrecipitationType = "sleet"
	PrecipitationSnow  PrecipitationType = "snow"
)

// PrecipitationIntensity is the closed amount classification.
type PrecipitationIntensity string

const (
	PrecipitationLight    PrecipitationIntensity = "light"
	PrecipitationModerate PrecipitationIntensity = "moderate"
	PrecipitationHeavy    PrecipitationIntensity = "heavy"
)

// AlertSeverity is the closed synthesized warning scale.
type AlertSeverity string

const (
	AlertSeverityModerate AlertSeverity = "moderate"
	AlertSeveritySevere   AlertSeverity = "severe"
	AlertSeverityExtreme  AlertSeverity = "extreme"
)

// AlertType identifies a synthesized warning category.
type AlertType string

const (
	AlertCold    AlertType = "cold"
	AlertHeat    AlertType = "heat"
	AlertSnow    AlertType = "snow"
	AlertStorm   AlertType = "storm"
	AlertTyphoon AlertType = "typhoon"
	AlertWind    AlertType = "wind"
)

// AirQualityLevel is the US AQI qualitative scale.
type AirQualityLevel string

const (
	AirQualityGood                        AirQualityLevel = "Good"
	AirQualityModerate                    AirQualityLevel = "Moderate"
	AirQualityUnhealthyForSensitiveGroups AirQualityLevel = "Unhealthy for Sensitive Groups"
	AirQualityUnhealthy                   AirQualityLevel = "Unhealthy"
	AirQualityVeryUnhealthy               AirQualityLevel = "Very Unhealthy"
	AirQualityHazardous                   AirQualityLevel = "Hazardous"
)

// UVLevel is the WHO UV-index qualitative scale.
type UVLevel string

const (
	UVLow      UVLevel = "Low"
	UVModerate UVLevel = "Moderate"
	UVHigh     UVLevel = "High"
	UVVeryHigh UVLevel = "Very High"
	UVExtreme  UVLevel = "Extreme"
)
