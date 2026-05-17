// Package alibaba wraps Alibaba Cloud's DashScope model platform,
// which hosts the Qwen / Tongyi family and several other Alibaba models.
//
// DashScope exposes two surfaces:
//   - the native /api/v1/services/aigc/text-generation/generation
//     endpoint with a DashScope-specific JSON shape (not used here);
//   - the /compatible-mode/v1 path which speaks the OpenAI
//     chat-completions / embeddings spec.
//
// This package uses the compatible-mode endpoint so we can route
// through the [openai] provider facade. DashScope-specific knobs
// (enable_thinking, enable_search, web search citations, etc.) ride
// through the Extra-threaded openai params.
//
// See https://help.aliyun.com/zh/model-studio/ for the docs.
package alibaba
