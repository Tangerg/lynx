package mistral

const (
	Provider = "Mistral"
)

const (
	OptionsKey     = "lynx:ai:model:mistral_options"
	DefaultBaseURL = "https://api.mistral.ai/v1"
)

// Chat model ids. See https://docs.mistral.ai/getting-started/models/models_overview/.
// The "-latest" aliases auto-track Mistral's current production
// snapshot; pin to a dated alias (e.g. mistral-large-2411) if you
// need reproducibility.
const (
	// ModelLarge (mistral-large-latest) is the top-tier flagship —
	// best reasoning quality, highest cost.
	ModelLarge = "mistral-large-latest"

	// ModelMedium (mistral-medium-latest) is the mid-tier model.
	ModelMedium = "mistral-medium-latest"

	// ModelSmall (mistral-small-latest) is the cheap / fast tier.
	ModelSmall = "mistral-small-latest"

	// ModelCodestral (codestral-latest) targets code generation,
	// FIM, and code reasoning.
	ModelCodestral = "codestral-latest"

	// ModelMinistral3B (ministral-3b-latest) is the 3B-param edge
	// model.
	ModelMinistral3B = "ministral-3b-latest"

	// ModelMinistral8B (ministral-8b-latest) is the 8B-param edge
	// model.
	ModelMinistral8B = "ministral-8b-latest"

	// ModelPixtralLarge (pixtral-large-latest) is the multimodal
	// vision-language flagship.
	ModelPixtralLarge = "pixtral-large-latest"

	// ModelMagistralMedium (magistral-medium-latest) is the medium
	// reasoning model — visible chain-of-thought via reasoning_content.
	ModelMagistralMedium = "magistral-medium-latest"

	// ModelMagistralSmall (magistral-small-latest) is the small
	// reasoning variant.
	ModelMagistralSmall = "magistral-small-latest"

	// ModelDevstralMedium (devstral-medium-latest) targets agentic
	// software-engineering workloads.
	ModelDevstralMedium = "devstral-medium-latest"
)

// Embedding model ids.
const (
	// ModelEmbed (mistral-embed) is the general-purpose embedding
	// model. 1024-dim by default; pass [embedding.Options.Dimensions]
	// to truncate.
	ModelEmbed = "mistral-embed"

	// ModelCodestralEmbed (codestral-embed) is the code-tuned
	// embedding model.
	ModelCodestralEmbed = "codestral-embed"
)

// Moderation model ids.
const (
	// ModelModeration (mistral-moderation-latest) is the moderation
	// classifier reachable via [NewModerationModel].
	ModelModeration = "mistral-moderation-latest"
)
