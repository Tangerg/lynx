package mistral

import (
	"context"
	"errors"
	"net/http"

	"github.com/Tangerg/lynx/core/moderation"
)

type ModerationModelConfig struct {
	APIKey         string
	DefaultOptions moderation.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (m ModerationModelConfig) Validate() error {
	if m.APIKey == "" {
		return errors.New("mistral: APIKey is required")
	}
	if m.DefaultOptions.Model == "" {
		return errors.New("mistral: DefaultOptions.Model is required")
	}
	if _, err := m.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ moderation.Model = (*ModerationModel)(nil)

// ModerationModel wraps Mistral's /moderations endpoint. Mistral
// reports a custom category set (sexual / hate_and_discrimination /
// violence_and_threats / dangerous_and_criminal_content / selfharm /
// health / financial / law / pii). Category names are preserved exactly.
type ModerationModel struct {
	api            *api
	defaultOptions moderation.Options
}

func NewModerationModel(cfg ModerationModelConfig) (*ModerationModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	api, err := newAPI(apiConfig{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, HTTPClient: cfg.HTTPClient})
	if err != nil {
		return nil, err
	}
	return &ModerationModel{api: api, defaultOptions: cfg.DefaultOptions.Clone()}, nil
}

func (m *ModerationModel) Call(ctx context.Context, req *moderation.Request) (*moderation.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	mergedOpts, err := m.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}

	apiResp, err := m.api.moderation(ctx, &moderationRequest{
		Model: mergedOpts.Model,
		Input: req.Texts,
	})
	if err != nil {
		return nil, err
	}

	outputs := make([]*moderation.Output, 0, len(apiResp.Results))
	for _, item := range apiResp.Results {
		cats := mapMistralCategories(item.Categories, item.CategoryScores)
		res, err := moderation.NewOutput(cats, &moderation.OutputMetadata{})
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, res)
	}

	meta := &moderation.ResponseMetadata{
		ID:    apiResp.ID,
		Model: apiResp.Model,
	}
	return moderation.NewResponse(outputs, meta)
}

func mapMistralCategories(flags map[string]bool, scores map[string]float64) moderation.Categories {
	categories := make(moderation.Categories, len(flags)+len(scores))
	for category, score := range scores {
		categories[category] = moderation.Verdict{Flagged: flags[category], Score: score}
	}
	for category, flagged := range flags {
		if _, ok := categories[category]; !ok {
			categories[category] = moderation.Verdict{Flagged: flagged}
		}
	}
	return categories
}
