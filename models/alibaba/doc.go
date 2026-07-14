// Package alibaba wraps Alibaba Cloud's DashScope model platform,
// which hosts the Qwen / Tongyi family and several other Alibaba models.
//
// DashScope exposes two surfaces:
//   - the native /api/v1/services/aigc/text-generation/generation
//     endpoint with a DashScope-specific JSON shape (not used here);
//   - the /compatible-mode/v1 path which speaks the OpenAI
//     chat-completions / embeddings spec.
//
// This package uses the compatible-mode endpoint to route
// through the [openai] provider facade. DashScope-specific knobs
// (enable_thinking, enable_search, web search citations, etc.) use the
// namespaced OpenAI request extension.
//
// See https://help.aliyun.com/zh/model-studio/ for the docs.
package alibaba
