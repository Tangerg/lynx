package assemblyai

import (
	"time"
)

const (
	Provider = "AssemblyAI"
)

const (
	OptionsKey = "lynx:ai:model:assemblyai_options"

	// DefaultBaseURL is AssemblyAI's production REST endpoint.
	DefaultBaseURL = "https://api.assemblyai.com/v2"

	// DefaultPollInterval is how often [AudioTranscriptionModel.Call]
	// re-checks a queued job. AssemblyAI's typical real-time-factor is
	// 0.1–0.3x audio length so 2s strikes a balance between latency and
	// API call volume.
	DefaultPollInterval = 2 * time.Second

	// DefaultPollTimeout caps the total wait for one Call. Long audio
	// (hour-scale lectures, podcasts) plus model warm-up can take a
	// while, so the ceiling is set generously; callers wanting a
	// tighter SLA should pass a ctx with their own deadline.
	DefaultPollTimeout = 30 * time.Minute
)
