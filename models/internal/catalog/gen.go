//go:build ignore

// Command gen regenerates the embedded model catalog (configs/*.json)
// from models.dev — a community model database (the same data behind
// LangChain's model profiles). Run it from this directory:
//
//	go run gen.go                          # fetch live https://models.dev/api.json
//	go run gen.go -source ./api.json       # or read a local snapshot
//
// Pipeline:
//
//  1. Load models.dev's api.json — a fully-resolved JSON map of
//     provider id -> { models: { model id -> spec } }. It's already
//     resolved (TOML parsed, [extends] applied), so this tool needs only
//     encoding/json and no TOML dependency.
//  2. Keep only the providers lynx ships a chat adapter for (providerMap),
//     and only chat models (drop embedding / TTS / image-generation —
//     output modality not "text", or an embedding family).
//  3. Map each spec into a chat.ModelInfo. The output is marshaled from
//     the real struct, so the generated JSON can never drift from the Go
//     type — that's the reason this is a Go program and not a script.
//  4. Overlay augmentations.json for fields models.dev lacks. Today that's
//     only reasoning effort levels (models.dev has a bare reasoning bool);
//     the file is the seam to hand-fill anything upstream is missing,
//     mirroring LangChain's profile_augmentations.
//
// To add a provider: add it to providerMap (left = models.dev provider id,
// right = the adapter's Provider const, lowercased) and re-run.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
)

const defaultSource = "https://models.dev/api.json"

// providerMap maps a models.dev provider id to our config/provider name
// (the adapter's Provider const, lowercased — Lookup matches case-folded).
// OpenAI-compat and managed delegators keep their own name: vertexai is
// sourced from google-vertex, zhipu from zhipuai.
var providerMap = map[string]string{
	"anthropic":      "anthropic",
	"openai":         "openai",
	"google":         "google",
	"google-vertex":  "vertexai",
	"deepseek":       "deepseek",
	"groq":           "groq",
	"xai":            "xai",
	"minimax":        "minimax",
	"zhipuai":        "zhipu",
	"alibaba":        "alibaba",
	"azure":          "azureopenai",
	"amazon-bedrock": "amazonbedrock",
	"fireworks-ai":   "fireworks",
	"huggingface":    "huggingface",
	"mistral":        "mistral",
	"moonshotai":     "moonshot",
	"ollama-cloud":   "ollama",
	"openrouter":     "openrouter",
	"perplexity":     "perplexity",
	"togetherai":     "together",
	"xiaomi":         "xiaomi",
}

// apiModel mirrors the subset of a models.dev model spec we consume.
type apiModel struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Family           string `json:"family"`
	Reasoning        bool   `json:"reasoning"`
	ToolCall         bool   `json:"tool_call"`
	StructuredOutput bool   `json:"structured_output"`
	Knowledge        string `json:"knowledge"`
	Modalities       struct {
		Input  []string `json:"input"`
		Output []string `json:"output"`
	} `json:"modalities"`
	Cost struct {
		Input      float64 `json:"input"`
		Output     float64 `json:"output"`
		CacheRead  float64 `json:"cache_read"`
		CacheWrite float64 `json:"cache_write"`
	} `json:"cost"`
	Limit struct {
		Context int64 `json:"context"`
		Input   int64 `json:"input"`
		Output  int64 `json:"output"`
	} `json:"limit"`
}

type apiProvider struct {
	Models map[string]apiModel `json:"models"`
}

// augEntry is a local augmentation overlay — fields models.dev lacks.
type augEntry struct {
	Levels       []string `json:"levels"`
	DefaultLevel string   `json:"default_level"`
}

// config is the on-disk shape of each configs/<provider>.json.
type config struct {
	Provider string           `json:"provider"`
	Models   []chat.ModelInfo `json:"models"`
}

func main() {
	source := flag.String("source", defaultSource, "models.dev api.json URL or local file path")
	flag.Parse()

	api, err := loadAPI(*source)
	if err != nil {
		fail("load %s: %v", *source, err)
	}
	augs, err := loadAugmentations("augmentations.json")
	if err != nil {
		fail("load augmentations: %v", err)
	}

	for apiID, provider := range providerMap {
		p, ok := api[apiID]
		if !ok {
			fail("provider %q not in source", apiID)
		}
		var models []chat.ModelInfo
		for id, m := range p.Models {
			if !isChat(m) {
				continue
			}
			models = append(models, toModelInfo(m, augs[provider][id]))
		}
		sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })

		out := filepath.Join("configs", provider+".json")
		if err := writeJSON(out, config{Provider: provider, Models: models}); err != nil {
			fail("write %s: %v", out, err)
		}
		fmt.Printf("%s: %d chat models\n", out, len(models))
	}
}

// isChat keeps only chat models — embeddings (by family/id) and models
// that don't emit text (TTS, image generation) are dropped.
func isChat(m apiModel) bool {
	lid, fam := strings.ToLower(m.ID), strings.ToLower(m.Family)
	if strings.Contains(fam, "embed") || strings.Contains(lid, "embed") || strings.Contains(lid, "rerank") {
		return false
	}
	if len(m.Modalities.Output) > 0 && !slices.Contains(m.Modalities.Output, "text") {
		return false
	}
	return true
}

func toModelInfo(m apiModel, aug augEntry) chat.ModelInfo {
	info := chat.ModelInfo{
		ID:               m.ID,
		DisplayName:      m.Name,
		KnowledgeCutoff:  m.Knowledge,
		ToolCall:         m.ToolCall,
		StructuredOutput: m.StructuredOutput,
		Pricing: chat.Pricing{
			InputPer1M:      m.Cost.Input,
			OutputPer1M:     m.Cost.Output,
			CacheReadPer1M:  m.Cost.CacheRead,
			CacheWritePer1M: m.Cost.CacheWrite,
		},
		Modalities: chat.Modalities{
			Input:  toModalities(m.Modalities.Input),
			Output: toModalities(m.Modalities.Output),
		},
		Limits: chat.Limits{
			ContextWindow:   m.Limit.Context,
			MaxInputTokens:  m.Limit.Input,
			MaxOutputTokens: m.Limit.Output,
		},
	}
	if m.Reasoning {
		// models.dev only knows whether a model reasons; effort levels
		// come from the augmentation file.
		info.Reasoning = chat.Reasoning{
			Supported:    true,
			Levels:       aug.Levels,
			DefaultLevel: aug.DefaultLevel,
		}
	}
	return info
}

func toModalities(in []string) []chat.Modality {
	if len(in) == 0 {
		return nil
	}
	out := make([]chat.Modality, len(in))
	for i, s := range in {
		out[i] = chat.Modality(s)
	}
	return out
}

func loadAPI(source string) (map[string]apiProvider, error) {
	var raw []byte
	var err error
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		resp, e := http.Get(source)
		if e != nil {
			return nil, e
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("status %s", resp.Status)
		}
		raw, err = io.ReadAll(resp.Body)
	} else {
		raw, err = os.ReadFile(source)
	}
	if err != nil {
		return nil, err
	}
	var api map[string]apiProvider
	if err := json.Unmarshal(raw, &api); err != nil {
		return nil, err
	}
	return api, nil
}

func loadAugmentations(path string) (map[string]map[string]augEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var augs map[string]map[string]augEntry
	if err := json.Unmarshal(raw, &augs); err != nil {
		return nil, err
	}
	return augs, nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "gen: "+format+"\n", args...)
	os.Exit(1)
}
