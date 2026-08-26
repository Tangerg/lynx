package perplexity

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/protocol/openai"
)

const RequestExtensionKey = "perplexity/request"

type SearchMode string

const (
	SearchModeWeb      SearchMode = "web"
	SearchModeAcademic SearchMode = "academic"
	SearchModeSEC      SearchMode = "sec"
)

// Valid reports whether s names a supported Sonar search mode.
func (s SearchMode) Valid() bool {
	return s == SearchModeWeb || s == SearchModeAcademic || s == SearchModeSEC
}

type SearchRecency string

const (
	SearchRecencyHour  SearchRecency = "hour"
	SearchRecencyDay   SearchRecency = "day"
	SearchRecencyWeek  SearchRecency = "week"
	SearchRecencyMonth SearchRecency = "month"
	SearchRecencyYear  SearchRecency = "year"
)

// Valid reports whether s names a supported search recency window.
func (s SearchRecency) Valid() bool {
	return s == SearchRecencyHour || s == SearchRecencyDay || s == SearchRecencyWeek ||
		s == SearchRecencyMonth || s == SearchRecencyYear
}

type SearchContextSize string

const (
	SearchContextLow    SearchContextSize = "low"
	SearchContextMedium SearchContextSize = "medium"
	SearchContextHigh   SearchContextSize = "high"
)

// Valid reports whether s names a supported search context size.
func (s SearchContextSize) Valid() bool {
	return s == SearchContextLow || s == SearchContextMedium || s == SearchContextHigh
}

type SearchType string

const (
	SearchTypeFast SearchType = "fast"
	SearchTypeAuto SearchType = "auto"
	SearchTypePro  SearchType = "pro"
)

// Valid reports whether s names a supported Pro Search routing mode.
func (s SearchType) Valid() bool {
	return s == SearchTypeFast || s == SearchTypeAuto || s == SearchTypePro
}

type ReasoningEffort string

const (
	ReasoningEffortMinimal ReasoningEffort = "minimal"
	ReasoningEffortLow     ReasoningEffort = "low"
	ReasoningEffortMedium  ReasoningEffort = "medium"
	ReasoningEffortHigh    ReasoningEffort = "high"
)

// Valid reports whether r names a supported reasoning effort.
func (r ReasoningEffort) Valid() bool {
	return r == ReasoningEffortMinimal || r == ReasoningEffortLow ||
		r == ReasoningEffortMedium || r == ReasoningEffortHigh
}

// SearchDate is a Sonar search boundary encoded as MM/DD/YYYY.
type SearchDate string

// Valid reports whether s is a concrete valid Sonar search date.
func (s SearchDate) Valid() bool {
	if s == "" {
		return false
	}
	_, err := time.Parse(searchDateLayout, string(s))
	return err == nil
}

func (s SearchDate) validate(field string) error {
	if s == "" || s.Valid() {
		return nil
	}
	_, err := time.Parse(searchDateLayout, string(s))
	return fmt.Errorf("%s must use MM/DD/YYYY: %w", field, err)
}

// ImageFormat names one image format accepted by Sonar image filtering.
type ImageFormat string

const (
	ImageFormatGIF  ImageFormat = "gif"
	ImageFormatJPG  ImageFormat = "jpg"
	ImageFormatJPEG ImageFormat = "jpeg"
	ImageFormatPNG  ImageFormat = "png"
	ImageFormatWebP ImageFormat = "webp"
)

// Valid reports whether i names a supported image format.
func (i ImageFormat) Valid() bool {
	return i == ImageFormatGIF || i == ImageFormatJPG || i == ImageFormatJPEG ||
		i == ImageFormatPNG || i == ImageFormatWebP
}

const searchDateLayout = "1/2/2006"

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
// equivalent. Store it in [chat.Options.Extensions] under RequestExtensionKey.
type RequestOptions struct {
	WebSearchOptions        *WebSearchOptions `json:"web_search_options,omitempty"`
	SearchMode              SearchMode        `json:"search_mode,omitempty"`
	ReturnImages            *bool             `json:"return_images,omitempty"`
	ReturnRelatedQuestions  *bool             `json:"return_related_questions,omitempty"`
	EnableSearchClassifier  *bool             `json:"enable_search_classifier,omitempty"`
	DisableSearch           *bool             `json:"disable_search,omitempty"`
	SearchDomainFilter      []string          `json:"search_domain_filter,omitempty"`
	SearchLanguageFilter    []string          `json:"search_language_filter,omitempty"`
	SearchRecencyFilter     SearchRecency     `json:"search_recency_filter,omitempty"`
	SearchAfterDateFilter   SearchDate        `json:"search_after_date_filter,omitempty"`
	SearchBeforeDateFilter  SearchDate        `json:"search_before_date_filter,omitempty"`
	LastUpdatedBeforeFilter SearchDate        `json:"last_updated_before_filter,omitempty"`
	LastUpdatedAfterFilter  SearchDate        `json:"last_updated_after_filter,omitempty"`
	ImageFormatFilter       []ImageFormat     `json:"image_format_filter,omitempty"`
	ImageDomainFilter       []string          `json:"image_domain_filter,omitempty"`
	ReasoningEffort         ReasoningEffort   `json:"reasoning_effort,omitempty"`
	LanguagePreference      string            `json:"language_preference,omitempty"`
}

func prepareRequest(source *corechat.Request, target *openai.CompatibleRequest) error {
	if err := validateCoreOptions(source.Options); err != nil {
		return err
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
	if err := options.ValidateFor(target.Model(), target.Stream()); err != nil {
		return err
	}
	encoded, err := json.Marshal(options)
	if err != nil {
		return fmt.Errorf("encode extension %q: %w", RequestExtensionKey, err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return fmt.Errorf("normalize extension %q: %w", RequestExtensionKey, err)
	}
	for field, value := range fields {
		if err := target.SetExtraField(field, value); err != nil {
			return err
		}
	}
	return nil
}

func decodeRequestOptions(request *corechat.Request) (RequestOptions, error) {
	fields, _, err := request.Options.Extensions.Decode[map[string]any](RequestExtensionKey)
	if err != nil {
		return RequestOptions{}, fmt.Errorf("extension %q: %w", RequestExtensionKey, err)
	}
	if _, exists := fields["response_format"]; exists {
		return RequestOptions{}, fmt.Errorf("extension %q field %q is owned by options.output_format", RequestExtensionKey, "response_format")
	}
	options, _, err := request.Options.Extensions.Decode[RequestOptions](RequestExtensionKey)
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

func (r RequestOptions) ValidateFor(model string, stream bool) error {
	if err := validateEnum("search_mode", r.SearchMode); err != nil {
		return err
	}
	if err := validateEnum("search_recency_filter", r.SearchRecencyFilter); err != nil {
		return err
	}
	if err := validateEnum("reasoning_effort", r.ReasoningEffort); err != nil {
		return err
	}
	if r.EnableSearchClassifier != nil && r.DisableSearch != nil && *r.EnableSearchClassifier && *r.DisableSearch {
		return errors.New("enable_search_classifier and disable_search cannot both be true")
	}
	if err := validateDomains("search_domain_filter", r.SearchDomainFilter, 20); err != nil {
		return err
	}
	if err := validateDomains("image_domain_filter", r.ImageDomainFilter, 10); err != nil {
		return err
	}
	if len(r.SearchLanguageFilter) > 10 {
		return errors.New("search_language_filter must contain at most 10 language codes")
	}
	for index, language := range r.SearchLanguageFilter {
		if !isLanguageCode(language) {
			return fmt.Errorf("search_language_filter[%d] must be a lowercase ISO 639-1 code", index)
		}
	}
	if r.LanguagePreference != "" && !isLanguageCode(r.LanguagePreference) {
		return errors.New("language_preference must be a lowercase ISO 639-1 code")
	}
	if err := r.validateDates(); err != nil {
		return err
	}
	if r.SearchMode == SearchModeAcademic && r.hasDateFilter() {
		return errors.New("date filters are not supported with academic search mode")
	}
	if len(r.ImageFormatFilter) > 10 {
		return errors.New("image_format_filter must contain at most 10 formats")
	}
	for index, format := range r.ImageFormatFilter {
		if !format.Valid() {
			return fmt.Errorf("image_format_filter[%d] has unsupported format %q", index, format)
		}
	}
	if (len(r.ImageFormatFilter) != 0 || len(r.ImageDomainFilter) != 0) && (r.ReturnImages == nil || !*r.ReturnImages) {
		return errors.New("image filters require return_images=true")
	}
	if r.WebSearchOptions != nil {
		if err := r.WebSearchOptions.ValidateFor(model, stream); err != nil {
			return fmt.Errorf("web_search_options: %w", err)
		}
	}
	return nil
}

func (w WebSearchOptions) ValidateFor(model string, stream bool) error {
	if err := validateEnum("search_context_size", w.SearchContextSize); err != nil {
		return err
	}
	if err := validateEnum("search_type", w.SearchType); err != nil {
		return err
	}
	if w.SearchType != "" && model != "" && model != ModelSonarPro {
		return fmt.Errorf("search_type is supported only by model %q", ModelSonarPro)
	}
	if (w.SearchType == SearchTypeAuto || w.SearchType == SearchTypePro) && !stream {
		return fmt.Errorf("search_type %q requires streaming", w.SearchType)
	}
	if w.UserLocation != nil {
		if err := w.UserLocation.Validate(); err != nil {
			return fmt.Errorf("user_location: %w", err)
		}
	}
	return nil
}

func (u UserLocation) Validate() error {
	if u.Country != "" && (len(u.Country) != 2 || strings.ToUpper(u.Country) != u.Country) {
		return errors.New("country must be an uppercase ISO 3166-1 alpha-2 code")
	}
	if (u.Latitude == nil) != (u.Longitude == nil) {
		return errors.New("latitude and longitude must be provided together")
	}
	if u.Latitude != nil {
		if u.Country == "" {
			return errors.New("country is required with coordinates")
		}
		if *u.Latitude < -90 || *u.Latitude > 90 {
			return errors.New("latitude must be between -90 and 90")
		}
		if *u.Longitude < -180 || *u.Longitude > 180 {
			return errors.New("longitude must be between -180 and 180")
		}
	}
	return nil
}

type validRequestEnum interface {
	~string
	Valid() bool
}

func validateEnum[Enum validRequestEnum](name string, value Enum) error {
	if value == "" {
		return nil
	}
	if value.Valid() {
		return nil
	}
	return fmt.Errorf("%s has unsupported value %q", name, value)
}

func (r RequestOptions) validateDates() error {
	if err := r.SearchAfterDateFilter.validate("search_after_date_filter"); err != nil {
		return err
	}
	if err := r.SearchBeforeDateFilter.validate("search_before_date_filter"); err != nil {
		return err
	}
	if err := r.LastUpdatedBeforeFilter.validate("last_updated_before_filter"); err != nil {
		return err
	}
	return r.LastUpdatedAfterFilter.validate("last_updated_after_filter")
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

func (r RequestOptions) hasDateFilter() bool {
	return r.SearchRecencyFilter != "" ||
		r.SearchAfterDateFilter != "" ||
		r.SearchBeforeDateFilter != "" ||
		r.LastUpdatedBeforeFilter != "" ||
		r.LastUpdatedAfterFilter != ""
}
