// Package openaicompat is a registry of base URLs for providers whose
// chat / embedding APIs follow OpenAI's spec but ship behind a
// different host AND don't have a dedicated facade package.
//
// Use these constants with [option.WithBaseURL] when constructing an
// [openai.Chat] / [openai.EmbeddingModel]. Providers with a
// dedicated facade package (deepseek, alibaba, huggingface, mistral,
// zhipu, minimax, moonshot, openrouter) own their own BaseURL
// constants under models/<provider> — go through the facade instead
// of importing them here, both to avoid duplication and to pick up
// typed extras (region routing, attribution headers, model-id
// constants, etc.).
//
// Naming convention: constants use the **provider / platform** brand,
// not the model brand. Where ambiguous, the API platform name is
// preferred over the company name (parallel to "bedrock" vs "aws"):
// VolcanoArk (not DouBao), Qianfan (not Wenxin), Lingyiwanwu (not Yi).
package openaicompat
