package openai

import (
	"context"
	"errors"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/moderation"
	"github.com/Tangerg/lynx/models/internal/options"
)

type ModerationModelConfig struct {
	APIKey         model.APIKey
	DefaultOptions *moderation.Options
	RequestOptions []option.RequestOption

	// Metadata overrides the [moderation.ModelMetadata] returned by [ModerationModel.Metadata].
	// Zero Provider falls back to [Provider].
	Metadata *moderation.ModelMetadata
}

func (c ModerationModelConfig) Validate() error {
	if c.APIKey == nil {
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
	metadata       moderation.ModelMetadata
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

	info := moderation.ModelMetadata{Provider: Provider}
	if cfg.Metadata != nil {
		info = *cfg.Metadata
	}
	return &ModerationModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
		metadata:       info,
	}, nil
}

func (m *ModerationModel) buildAPIModerationRequest(req *moderation.Request) (*openai.ModerationNewParams, error) {
	mergedOpts, err := moderation.MergeOptions(m.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	params := options.GetParams[openai.ModerationNewParams](mergedOpts, OptionsKey)

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

func (m *ModerationModel) DefaultOptions() moderation.Options {
	return *m.defaultOptions
}

func (m *ModerationModel) Metadata() moderation.ModelMetadata {
	return m.metadata
}
