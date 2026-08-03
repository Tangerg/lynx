package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/openai/openai-go/v3"

	"github.com/Tangerg/lynx/core/moderation"
)

type ModerationModelConfig struct {
	Provider       string
	APIKey         string
	DefaultOptions moderation.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ModerationModelConfig) Validate() error {
	if err := validateProvider(c.Provider); err != nil {
		return fmt.Errorf("openai: Provider: %w", err)
	}
	if c.APIKey == "" {
		return errors.New("openai: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("openai: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ moderation.Model = (*ModerationModel)(nil)

type ModerationModel struct {
	api            *api
	provider       string
	defaultOptions moderation.Options
}

func NewModerationModel(cfg ModerationModelConfig) (*ModerationModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := newAPI(apiConfig{
		APIKey:     cfg.APIKey,
		BaseURL:    cfg.BaseURL,
		HTTPClient: cfg.HTTPClient,
	})
	if err != nil {
		return nil, err
	}

	return &ModerationModel{
		api:            api,
		provider:       cfg.Provider,
		defaultOptions: cfg.DefaultOptions.Clone(),
	}, nil
}

func (m *ModerationModel) buildAPIModerationRequest(req *moderation.Request) (*openai.ModerationNewParams, error) {
	mergedOpts, err := m.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}

	fields, err := decodeRequestFields(mergedOpts.Extensions, protocolModalityRequestExtensionKey(m.provider, "moderation"), "model", "input")
	if err != nil {
		return nil, err
	}
	params := &openai.ModerationNewParams{}
	params.SetExtraFields(fields)

	params.Model = mergedOpts.Model
	params.Input = openai.ModerationNewParamsInputUnion{
		OfStringArray: req.Texts,
	}

	return params, nil
}

func (m *ModerationModel) buildModerationResponse(resp *openai.ModerationNewResponse) (*moderation.Response, error) {
	results := make([]*moderation.Result, 0, len(resp.Results))

	for _, item := range resp.Results {
		cats := moderation.Categories{
			"harassment": {
				Flagged: item.Categories.Harassment,
				Score:   item.CategoryScores.Harassment,
			},
			"harassment_threatening": {
				Flagged: item.Categories.HarassmentThreatening,
				Score:   item.CategoryScores.HarassmentThreatening,
			},
			"hate": {
				Flagged: item.Categories.Hate,
				Score:   item.CategoryScores.Hate,
			},
			"hate_threatening": {
				Flagged: item.Categories.HateThreatening,
				Score:   item.CategoryScores.HateThreatening,
			},
			"illicit": {
				Flagged: item.Categories.Illicit,
				Score:   item.CategoryScores.Illicit,
			},
			"illicit_violent": {
				Flagged: item.Categories.IllicitViolent,
				Score:   item.CategoryScores.IllicitViolent,
			},
			"self_harm": {
				Flagged: item.Categories.SelfHarm,
				Score:   item.CategoryScores.SelfHarm,
			},
			"self_harm_instructions": {
				Flagged: item.Categories.SelfHarmInstructions,
				Score:   item.CategoryScores.SelfHarmInstructions,
			},
			"self_harm_intent": {
				Flagged: item.Categories.SelfHarmIntent,
				Score:   item.CategoryScores.SelfHarmIntent,
			},
			"sexual": {
				Flagged: item.Categories.Sexual,
				Score:   item.CategoryScores.Sexual,
			},
			"sexual_minors": {
				Flagged: item.Categories.SexualMinors,
				Score:   item.CategoryScores.SexualMinors,
			},
			"violence": {
				Flagged: item.Categories.Violence,
				Score:   item.CategoryScores.Violence,
			},
			"violence_graphic": {
				Flagged: item.Categories.ViolenceGraphic,
				Score:   item.CategoryScores.ViolenceGraphic,
			},
		}

		result, err := moderation.NewResult(cats, &moderation.ResultMetadata{})
		if err != nil {
			return nil, err
		}

		results = append(results, result)
	}

	meta := &moderation.ResponseMetadata{
		ID:    resp.ID,
		Model: resp.Model,
	}

	return moderation.NewResponse(results, meta)
}

func (m *ModerationModel) Call(ctx context.Context, req *moderation.Request) (*moderation.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	apiReq, err := m.buildAPIModerationRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := m.api.moderation(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	return m.buildModerationResponse(apiResp)
}
