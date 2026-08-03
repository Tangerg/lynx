package bedrock

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

// Credentials configures explicit AWS credentials. Leave it nil to use the
// standard AWS credential chain (environment, shared config, IRSA, or IAM).
type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

type apiConfig struct {
	Region      string
	BaseURL     string
	HTTPClient  *http.Client
	Credentials *Credentials
}

func (c apiConfig) validate() error {
	if c.Credentials != nil {
		if c.Credentials.AccessKeyID == "" {
			return errors.New("bedrock: Credentials.AccessKeyID is required")
		}
		if c.Credentials.SecretAccessKey == "" {
			return errors.New("bedrock: Credentials.SecretAccessKey is required")
		}
	}
	return nil
}

type api struct {
	client *bedrockruntime.Client
}

func newAPI(ctx context.Context, cfg apiConfig) (*api, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	loadOptions := make([]func(*awsconfig.LoadOptions) error, 0, 3)
	if cfg.Region != "" {
		loadOptions = append(loadOptions, awsconfig.WithRegion(cfg.Region))
	}
	if cfg.HTTPClient != nil {
		loadOptions = append(loadOptions, awsconfig.WithHTTPClient(cfg.HTTPClient))
	}
	if cfg.Credentials != nil {
		loadOptions = append(loadOptions, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.Credentials.AccessKeyID,
			cfg.Credentials.SecretAccessKey,
			cfg.Credentials.SessionToken,
		)))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("bedrock: load AWS config: %w", err)
	}

	client := bedrockruntime.NewFromConfig(awsCfg, func(options *bedrockruntime.Options) {
		if cfg.BaseURL != "" {
			options.BaseEndpoint = aws.String(cfg.BaseURL)
		}
	})
	return &api{client: client}, nil
}

// Converse runs the unified inference API across every Bedrock-hosted
// model family (Claude / Llama / Titan / Mistral / Cohere / DeepSeek).
func (a *api) converse(ctx context.Context, params *bedrockruntime.ConverseInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
	if params == nil {
		return nil, errors.New("bedrock: request must not be nil")
	}
	return a.client.Converse(ctx, params, opts...)
}

// ConverseStream is the streaming variant. The event channel is on the
// returned EventStream — callers iterate via stream.Events() then
// stream.Close().
func (a *api) converseStream(ctx context.Context, params *bedrockruntime.ConverseStreamInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseStreamOutput, error) {
	if params == nil {
		return nil, errors.New("bedrock: request must not be nil")
	}
	return a.client.ConverseStream(ctx, params, opts...)
}

// InvokeModel is the raw per-model endpoint. Bedrock embeddings (Titan
// Embed v2, Cohere Embed v3, ...) only go through this — each family
// expects its own JSON body shape, so the lynx [EmbeddingModel] below
// branches by model family.
func (a *api) invokeModel(ctx context.Context, params *bedrockruntime.InvokeModelInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
	if params == nil {
		return nil, errors.New("bedrock: request must not be nil")
	}
	return a.client.InvokeModel(ctx, params, opts...)
}
