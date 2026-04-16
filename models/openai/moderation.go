package openai

import (
	"context"
	"errors"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/moderation"
)

type ModerationModelConfig struct {
	ApiKey         model.ApiKey
	DefaultOptions *moderation.Options
	RequestOptions []option.RequestOption
}

func (c *ModerationModelConfig) validate() error {
	if c == nil {
		return errors.New("openai: config is nil")
	}
	if c.ApiKey == nil {
		return errors.New("openai: api key is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("openai: default options are required")
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

	for _, item := range resp.Results {
		mod := &moderation.Moderation{
			Harassment: moderation.Category{
				Flagged: item.Categories.Harassment,
				Score:   item.CategoryScores.Harassment,
			},
			HarassmentThreatening: moderation.Category{
				Flagged: item.Categories.HarassmentThreatening,
				Score:   item.CategoryScores.HarassmentThreatening,
			},
			Hate: moderation.Category{
				Flagged: item.Categories.Hate,
				Score:   item.CategoryScores.Hate,
			},
			HateThreatening: moderation.Category{
				Flagged: item.Categories.HateThreatening,
				Score:   item.CategoryScores.HateThreatening,
			},
			Illicit: moderation.Category{
				Flagged: item.Categories.Illicit,
				Score:   item.CategoryScores.Illicit,
			},
			IllicitViolent: moderation.Category{
				Flagged: item.Categories.IllicitViolent,
				Score:   item.CategoryScores.IllicitViolent,
			},
			SelfHarm: moderation.Category{
				Flagged: item.Categories.SelfHarm,
				Score:   item.CategoryScores.SelfHarm,
			},
			SelfHarmInstructions: moderation.Category{
				Flagged: item.Categories.SelfHarmInstructions,
				Score:   item.CategoryScores.SelfHarmInstructions,
			},
			SelfHarmIntent: moderation.Category{
				Flagged: item.Categories.SelfHarmIntent,
				Score:   item.CategoryScores.SelfHarmIntent,
			},
			Sexual: moderation.Category{
				Flagged: item.Categories.Sexual,
				Score:   item.CategoryScores.Sexual,
			},
			SexualMinors: moderation.Category{
				Flagged: item.Categories.SexualMinors,
				Score:   item.CategoryScores.SexualMinors,
			},
			Violence: moderation.Category{
				Flagged: item.Categories.Violence,
				Score:   item.CategoryScores.Violence,
			},
			ViolenceGraphic: moderation.Category{
				Flagged: item.Categories.ViolenceGraphic,
				Score:   item.CategoryScores.ViolenceGraphic,
			},
		}

		result, err := moderation.NewResult(mod, &moderation.ResultMetadata{})
		if err != nil {
			return nil, err
		}

		results = append(results, result)
	}

	meta := &moderation.ResponseMetadata{
		ID:      resp.ID,
		Model:   resp.Model,
		Created: time.Now().Unix(),
	}

	return moderation.NewResponse(results, meta)
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
