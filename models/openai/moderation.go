package openai

import (
	"context"
	"errors"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/moderation"
	"github.com/Tangerg/lynx/models/internal/options"
)

type ModerationModelConfig struct {
	APIKey         string
	DefaultOptions *moderation.Options
	RequestOptions []option.RequestOption
}

func (c ModerationModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("openai: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("openai: DefaultOptions is required")
	}
	return nil
}

var _ moderation.Model = (*ModerationModel)(nil)

type ModerationModel struct {
	api            *API
	defaultOptions *moderation.Options
}

func NewModerationModel(cfg ModerationModelConfig) (*ModerationModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := NewAPI(APIConfig{
		APIKey:         cfg.APIKey,
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

func (m *ModerationModel) buildAPIModerationRequest(req *moderation.Request) (*openai.ModerationNewParams, error) {
	mergedOpts, err := moderation.MergeOptions(m.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	params, err := options.GetParams[openai.ModerationNewParams](mergedOpts.Extra, OptionsKey)
	if err != nil {
		return nil, err
	}

	params.Model = mergedOpts.Model
	params.Input = openai.ModerationNewParamsInputUnion{
		OfStringArray: req.Texts,
	}

	return params, nil
}

func (m *ModerationModel) buildModerationResponse(resp *openai.ModerationNewResponse) (*moderation.Response, error) {
	results := make([]*moderation.Result, 0, len(resp.Results))

	for _, item := range resp.Results {
		cats := &moderation.Categories{
			Harassment: moderation.Verdict{
				Flagged: item.Categories.Harassment,
				Score:   item.CategoryScores.Harassment,
			},
			HarassmentThreatening: moderation.Verdict{
				Flagged: item.Categories.HarassmentThreatening,
				Score:   item.CategoryScores.HarassmentThreatening,
			},
			Hate: moderation.Verdict{
				Flagged: item.Categories.Hate,
				Score:   item.CategoryScores.Hate,
			},
			HateThreatening: moderation.Verdict{
				Flagged: item.Categories.HateThreatening,
				Score:   item.CategoryScores.HateThreatening,
			},
			Illicit: moderation.Verdict{
				Flagged: item.Categories.Illicit,
				Score:   item.CategoryScores.Illicit,
			},
			IllicitViolent: moderation.Verdict{
				Flagged: item.Categories.IllicitViolent,
				Score:   item.CategoryScores.IllicitViolent,
			},
			SelfHarm: moderation.Verdict{
				Flagged: item.Categories.SelfHarm,
				Score:   item.CategoryScores.SelfHarm,
			},
			SelfHarmInstructions: moderation.Verdict{
				Flagged: item.Categories.SelfHarmInstructions,
				Score:   item.CategoryScores.SelfHarmInstructions,
			},
			SelfHarmIntent: moderation.Verdict{
				Flagged: item.Categories.SelfHarmIntent,
				Score:   item.CategoryScores.SelfHarmIntent,
			},
			Sexual: moderation.Verdict{
				Flagged: item.Categories.Sexual,
				Score:   item.CategoryScores.Sexual,
			},
			SexualMinors: moderation.Verdict{
				Flagged: item.Categories.SexualMinors,
				Score:   item.CategoryScores.SexualMinors,
			},
			Violence: moderation.Verdict{
				Flagged: item.Categories.Violence,
				Score:   item.CategoryScores.Violence,
			},
			ViolenceGraphic: moderation.Verdict{
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
		ID:      resp.ID,
		Model:   resp.Model,
		Created: time.Now().Unix(),
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

	apiResp, err := m.api.Moderation(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	return m.buildModerationResponse(apiResp)
}
