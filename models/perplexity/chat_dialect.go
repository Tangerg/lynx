package perplexity

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/metadata"
)

const RequestExtensionKey = "perplexity/request"

type SearchMode string

const (
	SearchModeWeb      SearchMode = "web"
	SearchModeAcademic SearchMode = "academic"
	SearchModeSEC      SearchMode = "sec"
)

type SearchRecency string

const (
	SearchRecencyHour  SearchRecency = "hour"
	SearchRecencyDay   SearchRecency = "day"
	SearchRecencyWeek  SearchRecency = "week"
	SearchRecencyMonth SearchRecency = "month"
	SearchRecencyYear  SearchRecency = "year"
)

type SearchContextSize string

const (
	SearchContextLow    SearchContextSize = "low"
	SearchContextMedium SearchContextSize = "medium"
	SearchContextHigh   SearchContextSize = "high"
)

type SearchType string

const (
	SearchTypeFast SearchType = "fast"
	SearchTypeAuto SearchType = "auto"
	SearchTypePro  SearchType = "pro"
)

type ReasoningEffort string

const (
	ReasoningEffortMinimal ReasoningEffort = "minimal"
	ReasoningEffortLow     ReasoningEffort = "low"
	ReasoningEffortMedium  ReasoningEffort = "medium"
	ReasoningEffortHigh    ReasoningEffort = "high"
)

// UserLocation biases Sonar search toward one geographic location.
type UserLocation struct {
	Country   string   `json:"country,omitempty"`
	Region    string   `json:"region,omitempty"`
	City      string   `json:"city,omitempty"`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
}

// WebSearchOptions controls Sonar's retrieval depth and Pro Search routing.
type WebSearchOptions struct {
	SearchContextSize SearchContextSize `json:"search_context_size,omitempty"`
	SearchType        SearchType        `json:"search_type,omitempty"`
	UserLocation      *UserLocation     `json:"user_location,omitempty"`
}

// RequestOptions contains the documented Sonar fields without a neutral Core
// equivalent. Store it in [chat.Request.Extensions] under RequestExtensionKey.
type RequestOptions struct {
	ResponseFormat          json.RawMessage   `json:"response_format,omitempty"`
	WebSearchOptions        *WebSearchOptions `json:"web_search_options,omitempty"`
	SearchMode              SearchMode        `json:"search_mode,omitempty"`
	ReturnImages            *bool             `json:"return_images,omitempty"`
	ReturnRelatedQuestions  *bool             `json:"return_related_questions,omitempty"`
	EnableSearchClassifier  *bool             `json:"enable_search_classifier,omitempty"`
	DisableSearch           *bool             `json:"disable_search,omitempty"`
	SearchDomainFilter      []string          `json:"search_domain_filter,omitempty"`
	SearchLanguageFilter    []string          `json:"search_language_filter,omitempty"`
	SearchRecencyFilter     SearchRecency     `json:"search_recency_filter,omitempty"`
	SearchAfterDateFilter   string            `json:"search_after_date_filter,omitempty"`
	SearchBeforeDateFilter  string            `json:"search_before_date_filter,omitempty"`
	LastUpdatedBeforeFilter string            `json:"last_updated_before_filter,omitempty"`
	LastUpdatedAfterFilter  string            `json:"last_updated_after_filter,omitempty"`
	ImageFormatFilter       []string          `json:"image_format_filter,omitempty"`
	ImageDomainFilter       []string          `json:"image_domain_filter,omitempty"`
	ReasoningEffort         ReasoningEffort   `json:"reasoning_effort,omitempty"`
	LanguagePreference      string            `json:"language_preference,omitempty"`
}

type sonarDialect struct{}

func (sonarDialect) PrepareRequest(source *corechat.Request, target *openaisdk.ChatCompletionNewParams) error {
	if err := validateCoreOptions(source.Options); err != nil {
		return err
	}
	if target.FrequencyPenalty.Valid() || target.PresencePenalty.Valid() {
		return errors.New("frequency_penalty and presence_penalty are not supported by the Sonar API")
	}
	if len(source.Tools) != 0 {
		return errors.New("custom function tools are not supported by the Sonar API; use Perplexity Agent API for tool orchestration")
	}
	for messageIndex := range source.Messages {
		message := source.Messages[messageIndex]
		if message.Role == corechat.RoleTool {
			return fmt.Errorf("messages[%d]: tool messages are not supported by the Sonar API", messageIndex)
		}
		for partIndex := range message.Parts {
			if message.Parts[partIndex].Kind != corechat.PartText {
				return fmt.Errorf("messages[%d].parts[%d]: Sonar supports text parts, got %q", messageIndex, partIndex, message.Parts[partIndex].Kind)
			}
		}
	}
	options, err := decodeRequestOptions(source)
	if err != nil {
		return err
	}
	return options.validate(string(target.Model), true)
}

func (sonarDialect) PrepareRequestOptions(source *corechat.Request, stream bool) ([]option.RequestOption, error) {
	request, err := decodeRequestOptions(source)
	if err != nil {
		return nil, err
	}
	if err := request.validate(source.Options.Model, stream); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode extension %q: %w", RequestExtensionKey, err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, fmt.Errorf("normalize extension %q: %w", RequestExtensionKey, err)
	}
	requestOptions := make([]option.RequestOption, 0, len(fields))
	for field, value := range fields {
		requestOptions = append(requestOptions, option.WithJSONSet(field, value))
	}
	return requestOptions, nil
}

func decodeRequestOptions(request *corechat.Request) (RequestOptions, error) {
	options, _, err := metadata.Decode[RequestOptions](request.Extensions, RequestExtensionKey)
	if err != nil {
		return RequestOptions{}, fmt.Errorf("extension %q: %w", RequestExtensionKey, err)
	}
	return options, nil
}

func validateCoreOptions(options corechat.Options) error {
	if options.FrequencyPenalty != nil {
		return errors.New("options.frequency_penalty is not supported by the Sonar API")
	}
	if options.PresencePenalty != nil {
		return errors.New("options.presence_penalty is not supported by the Sonar API")
	}
	if options.TopK != nil {
		return errors.New("options.top_k is not supported by the Sonar API")
	}
	return nil
}

func (options RequestOptions) validate(model string, stream bool) error {
	if len(options.ResponseFormat) != 0 && !json.Valid(options.ResponseFormat) {
		return errors.New("response_format contains invalid JSON")
	}
	if err := validateEnum("search_mode", string(options.SearchMode), "web", "academic", "sec"); err != nil {
		return err
	}
	if err := validateEnum("search_recency_filter", string(options.SearchRecencyFilter), "hour", "day", "week", "month", "year"); err != nil {
		return err
	}
	if err := validateEnum("reasoning_effort", string(options.ReasoningEffort), "minimal", "low", "medium", "high"); err != nil {
		return err
	}
	if options.EnableSearchClassifier != nil && options.DisableSearch != nil && *options.EnableSearchClassifier && *options.DisableSearch {
		return errors.New("enable_search_classifier and disable_search cannot both be true")
	}
	if err := validateDomains("search_domain_filter", options.SearchDomainFilter, 20); err != nil {
		return err
	}
	if err := validateDomains("image_domain_filter", options.ImageDomainFilter, 10); err != nil {
		return err
	}
	if len(options.SearchLanguageFilter) > 10 {
		return errors.New("search_language_filter must contain at most 10 language codes")
	}
	for index, language := range options.SearchLanguageFilter {
		if !isLanguageCode(language) {
			return fmt.Errorf("search_language_filter[%d] must be a lowercase ISO 639-1 code", index)
		}
	}
	if options.LanguagePreference != "" && !isLanguageCode(options.LanguagePreference) {
		return errors.New("language_preference must be a lowercase ISO 639-1 code")
	}
	for name, value := range map[string]string{
		"search_after_date_filter":   options.SearchAfterDateFilter,
		"search_before_date_filter":  options.SearchBeforeDateFilter,
		"last_updated_before_filter": options.LastUpdatedBeforeFilter,
		"last_updated_after_filter":  options.LastUpdatedAfterFilter,
	} {
		if value != "" {
			if _, err := time.Parse("1/2/2006", value); err != nil {
				return fmt.Errorf("%s must use MM/DD/YYYY: %w", name, err)
			}
		}
	}
	if options.SearchMode == SearchModeAcademic && hasDateFilter(options) {
		return errors.New("date filters are not supported with academic search mode")
	}
	if len(options.ImageFormatFilter) > 10 {
		return errors.New("image_format_filter must contain at most 10 formats")
	}
	for index, format := range options.ImageFormatFilter {
		switch format {
		case "gif", "jpg", "jpeg", "png", "webp":
		default:
			return fmt.Errorf("image_format_filter[%d] has unsupported format %q", index, format)
		}
	}
	if (len(options.ImageFormatFilter) != 0 || len(options.ImageDomainFilter) != 0) && (options.ReturnImages == nil || !*options.ReturnImages) {
		return errors.New("image filters require return_images=true")
	}
	if options.WebSearchOptions != nil {
		if err := options.WebSearchOptions.validate(model, stream); err != nil {
			return fmt.Errorf("web_search_options: %w", err)
		}
	}
	return nil
}

func (options WebSearchOptions) validate(model string, stream bool) error {
	if err := validateEnum("search_context_size", string(options.SearchContextSize), "low", "medium", "high"); err != nil {
		return err
	}
	if err := validateEnum("search_type", string(options.SearchType), "fast", "auto", "pro"); err != nil {
		return err
	}
	if options.SearchType != "" && model != "" && model != ModelSonarPro {
		return fmt.Errorf("search_type is supported only by model %q", ModelSonarPro)
	}
	if (options.SearchType == SearchTypeAuto || options.SearchType == SearchTypePro) && !stream {
		return fmt.Errorf("search_type %q requires streaming", options.SearchType)
	}
	if options.UserLocation != nil {
		if err := options.UserLocation.validate(); err != nil {
			return fmt.Errorf("user_location: %w", err)
		}
	}
	return nil
}

func (location UserLocation) validate() error {
	if location.Country != "" && (len(location.Country) != 2 || strings.ToUpper(location.Country) != location.Country) {
		return errors.New("country must be an uppercase ISO 3166-1 alpha-2 code")
	}
	if (location.Latitude == nil) != (location.Longitude == nil) {
		return errors.New("latitude and longitude must be provided together")
	}
	if location.Latitude != nil {
		if location.Country == "" {
			return errors.New("country is required with coordinates")
		}
		if *location.Latitude < -90 || *location.Latitude > 90 {
			return errors.New("latitude must be between -90 and 90")
		}
		if *location.Longitude < -180 || *location.Longitude > 180 {
			return errors.New("longitude must be between -180 and 180")
		}
	}
	return nil
}

func validateEnum(name, value string, allowed ...string) error {
	if value == "" {
		return nil
	}
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return fmt.Errorf("%s has unsupported value %q", name, value)
}

func validateDomains(name string, domains []string, limit int) error {
	if len(domains) > limit {
		return fmt.Errorf("%s must contain at most %d domains", name, limit)
	}
	denylist := false
	allowlist := false
	for index, domain := range domains {
		candidate := strings.TrimPrefix(domain, "-")
		if candidate == "" || strings.Contains(candidate, "://") || strings.ContainsAny(candidate, "/?# ") {
			return fmt.Errorf("%s[%d] must be a domain without scheme or path", name, index)
		}
		if strings.HasPrefix(domain, "-") {
			denylist = true
		} else {
			allowlist = true
		}
	}
	if denylist && allowlist {
		return fmt.Errorf("%s cannot mix allowlist and denylist entries", name)
	}
	return nil
}

func isLanguageCode(value string) bool {
	return len(value) == 2 && value[0] >= 'a' && value[0] <= 'z' && value[1] >= 'a' && value[1] <= 'z'
}

func hasDateFilter(options RequestOptions) bool {
	return options.SearchRecencyFilter != "" ||
		options.SearchAfterDateFilter != "" ||
		options.SearchBeforeDateFilter != "" ||
		options.LastUpdatedBeforeFilter != "" ||
		options.LastUpdatedAfterFilter != ""
}
