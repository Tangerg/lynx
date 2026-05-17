package bedrock

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

// ApiConfig configures the Bedrock Runtime client. Unlike single-provider
// SDKs, Bedrock authenticates via the standard AWS credential chain
// (env vars, ~/.aws/config, IRSA, IAM role) — so the typical config
// supplies just the region. Pass [AWSConfig] when callers want to use
// a pre-built aws.Config from their own SDK setup.
type ApiConfig struct {
	// Region overrides the AWS_REGION env var.
	Region string

	// AWSConfig lets callers thread a pre-built aws.Config (with their
	// own credentials provider, retryer, HTTP client, custom endpoint
	// resolver, etc.). When nil we load defaults via LoadDefaultConfig.
	AWSConfig *aws.Config
}

func (c *ApiConfig) validate() error {
	if c == nil {
		return errors.New("bedrock: config must not be nil")
	}
	return nil
}

type Api struct {
	client *bedrockruntime.Client
}

func NewApi(ctx context.Context, cfg *ApiConfig) (*Api, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	var awsCfg aws.Config
	if cfg.AWSConfig != nil {
		awsCfg = *cfg.AWSConfig
	} else {
		loaded, err := awsconfig.LoadDefaultConfig(ctx)
		if err != nil {
			return nil, err
		}
		awsCfg = loaded
	}
	if cfg.Region != "" {
		awsCfg.Region = cfg.Region
	}

	return &Api{client: bedrockruntime.NewFromConfig(awsCfg)}, nil
}

// Converse runs the unified inference API across every Bedrock-hosted
// model family (Claude / Llama / Titan / Mistral / Cohere / DeepSeek).
func (a *Api) Converse(ctx context.Context, params *bedrockruntime.ConverseInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
	if params == nil {
		return nil, errors.New("bedrock: request must not be nil")
	}
	return a.client.Converse(ctx, params, opts...)
}

// ConverseStream is the streaming variant. The event channel is on the
// returned EventStream — callers iterate via stream.Events() then
// stream.Close().
func (a *Api) ConverseStream(ctx context.Context, params *bedrockruntime.ConverseStreamInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseStreamOutput, error) {
	if params == nil {
		return nil, errors.New("bedrock: request must not be nil")
	}
	return a.client.ConverseStream(ctx, params, opts...)
}

// InvokeModel is the raw per-model endpoint. Bedrock embeddings (Titan
// Embed v2, Cohere Embed v3, ...) only go through this — each family
// expects its own JSON body shape, so the lynx [EmbeddingModel] below
// branches by model family.
func (a *Api) InvokeModel(ctx context.Context, params *bedrockruntime.InvokeModelInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
	if params == nil {
		return nil, errors.New("bedrock: request must not be nil")
	}
	return a.client.InvokeModel(ctx, params, opts...)
}
