package openai

import (
	"context"
	"errors"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/moderation"
)

type ModerationModelConfig struct {
	ApiKey         model.ApiKey
	DefaultOptions *moderation.Options
	RequestOptions []option.RequestOption
}

func (c *ModerationModelConfig) validate() error {
	if c == nil {
		return errors.New("config is nil")
	}
	if c.ApiKey == nil {
		return errors.New("apiKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("default options cannot be nil")
	}
	return nil
}

var _ moderation.Model = (*ModerationModel)(nil)

type ModerationModel struct {
	api            *Api
	defaultOptions *moderation.Options
}

func NewModerationModel(cfg *ModerationModelConfig) (*ModerationModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	api, err := NewApi(&ApiConfig{
		ApiKey:         cfg.ApiKey,
		RequestOptions: cfg.RequestOptions,
	})
	if err != nil {
		return nil, err
	}
	return &ModerationModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
	}, nil
}

func (m *ModerationModel) buildApiModerationRequest(req *moderation.Request) (*openai.ModerationNewParams, error) {
	mergedOpts, err := moderation.MergeOptions(m.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	params := getOptionsParams[openai.ModerationNewParams](mergedOpts)

	params.Model = mergedOpts.Model

	params.Input = openai.ModerationNewParamsInputUnion{
		OfStringArray: req.Texts,
	}

	return params, nil
}

func (m *ModerationModel) buildModerationResponse(resp *openai.ModerationNewResponse) (*moderation.Response, error) {
	results := make([]*moderation.Result, 0, len(resp.Results))
	for _, result := range resp.Results {
		mod := &moderation.Moderation{
			Harassment: moderation.Category{
				Flagged: result.Categories.Harassment,
				Score:   result.CategoryScores.Harassment,
			},
			HarassmentThreatening: moderation.Category{
				Flagged: result.Categories.HarassmentThreatening,
				Score:   result.CategoryScores.HarassmentThreatening,
			},
			Hate: moderation.Category{
				Flagged: result.Categories.Hate,
				Score:   result.CategoryScores.Hate,
			},
			HateThreatening: moderation.Category{
				Flagged: result.Categories.HateThreatening,
				Score:   result.CategoryScores.HateThreatening,
			},
			Illicit: moderation.Category{
				Flagged: result.Categories.Illicit,
				Score:   result.CategoryScores.Illicit,
			},
			IllicitViolent: moderation.Category{
				Flagged: result.Categories.IllicitViolent,
				Score:   result.CategoryScores.IllicitViolent,
			},
			SelfHarm: moderation.Category{
				Flagged: result.Categories.SelfHarm,
				Score:   result.CategoryScores.SelfHarm,
			},
			SelfHarmInstructions: moderation.Category{
				Flagged: result.Categories.SelfHarmInstructions,
				Score:   result.CategoryScores.SelfHarmInstructions,
			},
			SelfHarmIntent: moderation.Category{
				Flagged: result.Categories.SelfHarmIntent,
				Score:   result.CategoryScores.SelfHarmIntent,
			},
			Sexual: moderation.Category{
				Flagged: result.Categories.Sexual,
				Score:   result.CategoryScores.Sexual,
			},
			SexualMinors: moderation.Category{
				Flagged: result.Categories.SexualMinors,
				Score:   result.CategoryScores.SexualMinors,
			},
			Violence: moderation.Category{
				Flagged: result.Categories.Violence,
				Score:   result.CategoryScores.Violence,
			},
			ViolenceGraphic: moderation.Category{
				Flagged: result.Categories.ViolenceGraphic,
				Score:   result.CategoryScores.ViolenceGraphic,
			},
		}
		newResult, err := moderation.NewResult(mod, &moderation.ResultMetadata{})
		if err != nil {
			return nil, err
		}
		results = append(results, newResult)
	}
	respMeta := &moderation.ResponseMetadata{
		ID:      resp.ID,
		Model:   resp.Model,
		Created: time.Now().Unix(),
	}
	return moderation.NewResponse(results, respMeta)
}

func (m *ModerationModel) Call(ctx context.Context, req *moderation.Request) (*moderation.Response, error) {
	apiReq, err := m.buildApiModerationRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := m.api.Moderation(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	return m.buildModerationResponse(apiResp)
}

func (m *ModerationModel) DefaultOptions() *moderation.Options {
	return m.defaultOptions
}

func (m *ModerationModel) Info() moderation.ModelInfo {
	return moderation.ModelInfo{
		Provider: Provider,
	}
}
