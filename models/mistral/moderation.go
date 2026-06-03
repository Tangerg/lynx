package mistral

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/moderation"
)

type ModerationModelConfig struct {
	APIKey         model.APIKey
	DefaultOptions *moderation.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ModerationModelConfig) Validate() error {
	if c.APIKey == nil {
		return errors.New("mistral: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("mistral: DefaultOptions is required")
	}
	return nil
}

var _ moderation.Model = (*ModerationModel)(nil)

// ModerationModel wraps Mistral's /moderations endpoint. Mistral
// reports a custom category set (sexual / hate_and_discrimination /
// violence_and_threats / dangerous_and_criminal_content / selfharm /
// health / financial / law / pii); we map them onto lynx's typed
// [moderation.Moderation] slots so callers writing provider-agnostic
// policy code still see flags / scores in the standard fields.
type ModerationModel struct {
	api            *API
	defaultOptions *moderation.Options
}

func NewModerationModel(cfg ModerationModelConfig) (*ModerationModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	api, err := NewAPI(APIConfig{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, HTTPClient: cfg.HTTPClient})
	if err != nil {
		return nil, err
	}
	return &ModerationModel{api: api, defaultOptions: cfg.DefaultOptions}, nil
}

func (m *ModerationModel) Call(ctx context.Context, req *moderation.Request) (*moderation.Response, error) {
	mergedOpts, err := moderation.MergeOptions(m.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	apiResp, err := m.api.Moderation(ctx, &ModerationRequest{
		Model: mergedOpts.Model,
		Input: req.Texts,
	})
	if err != nil {
		return nil, err
	}

	results := make([]*moderation.Result, 0, len(apiResp.Results))
	for _, item := range apiResp.Results {
		mod := mapMistralCategories(item.Categories, item.CategoryScores)
		res, err := moderation.NewResult(mod, &moderation.ResultMetadata{})
		if err != nil {
			return nil, err
		}
		results = append(results, res)
	}

	meta := &moderation.ResponseMetadata{
		ID:      apiResp.ID,
		Model:   apiResp.Model,
		Created: time.Now().Unix(),
	}
	return moderation.NewResponse(results, meta)
}

// mapMistralCategories maps Mistral's category names onto lynx's
// typed Moderation slots. Mistral has a few categories OpenAI's set
// doesn't (health, financial, law) — those map directly. Categories
// Mistral doesn't surface stay zero.
func mapMistralCategories(flags map[string]bool, scores map[string]float64) *moderation.Moderation {
	get := func(key string) moderation.Category {
		return moderation.Category{Flagged: flags[key], Score: scores[key]}
	}
	return &moderation.Moderation{
		Sexual:                      get("sexual"),
		Hate:                        get("hate_and_discrimination"),
		Violence:                    get("violence_and_threats"),
		DangerousAndCriminalContent: get("dangerous_and_criminal_content"),
		SelfHarm:                    get("selfharm"),
		Health:                      get("health"),
		Financial:                   get("financial"),
		Law:                         get("law"),
		Pii:                         get("pii"),
	}
}

func (m *ModerationModel) DefaultOptions() moderation.Options { return *m.defaultOptions }
func (m *ModerationModel) Metadata() moderation.ModelMetadata {
	return moderation.ModelMetadata{Provider: Provider}
}
