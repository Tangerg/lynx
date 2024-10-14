package splitter

import (
	"context"
	"github.com/Tangerg/lynx/ai/core/document"
	"github.com/Tangerg/lynx/ai/core/tokenizer"
	"testing"
)

const content = `GPT-4o has safety built-in by design across modalities, through techniques such as filtering training data and refining the model’s behavior through post-training. We have also created new safety systems to provide guardrails on voice outputs.

We’ve evaluated GPT-4o according to our Preparedness Framework and in line with our voluntary commitments. Our evaluations of cybersecurity, CBRN, persuasion, and model autonomy show that GPT-4o does not score above Medium risk in any of these categories. This assessment involved running a suite of automated and human evaluations throughout the model training process. We tested both pre-safety-mitigation and post-safety-mitigation versions of the model, using custom fine-tuning and prompts, to better elicit model capabilities.

GPT-4o has also undergone extensive external red teaming with 70+ external experts in domains such as social psychology, bias and fairness, and misinformation to identify risks that are introduced or amplified by the newly added modalities. We used these learnings to build out our safety interventions in order to improve the safety of interacting with GPT-4o. We will continue to mitigate new risks as they’re discovered.

We recognize that GPT-4o’s audio modalities present a variety of novel risks. Today we are publicly releasing text and image inputs and text outputs. Over the upcoming weeks and months, we’ll be working on the technical infrastructure, usability via post-training, and safety necessary to release the other modalities. For example, at launch, audio outputs will be limited to a selection of preset voices and will abide by our existing safety policies. We will share further details addressing the full range of GPT-4o’s modalities in the forthcoming system card.

Through our testing and iteration with the model, we have observed several limitations that exist across all of the model’s modalities, a few of which are illustrated below.

GPT-4o is our latest step in pushing the boundaries of deep learning, this time in the direction of practical usability. We spent a lot of effort over the last two years working on efficiency improvements at every layer of the stack. As a first fruit of this research, we’re able to make a GPT-4 level model available much more broadly. GPT-4o’s capabilities will be rolled out iteratively (with extended red team access starting today). 

GPT-4o’s text and image capabilities are starting to roll out today in ChatGPT. We are making GPT-4o available in the free tier, and to Plus users with up to 5x higher message limits. We'll roll out a new version of Voice Mode with GPT-4o in alpha within ChatGPT Plus in the coming weeks.

Developers can also now access GPT-4o in the API as a text and vision model. GPT-4o is 2x faster, half the price, and has 5x higher rate limits compared to GPT-4 Turbo. We plan to launch support for GPT-4o's new audio and video capabilities to a small group of trusted partners in the API in the coming weeks.


`

func TestTokenSplitter(t *testing.T) {
	tiktoken, _ := tokenizer.NewTiktoken("o200k_base")
	ts := NewTokenSplitterBuilder(tiktoken).WithDefaultChunkSize(100).Build()
	doc := document.
		NewBuilder().
		WithContent(content).
		Build()
	transDocs, _ := ts.Transform(context.Background(), []*document.Document{doc})
	for _, transDoc := range transDocs {
		t.Log(transDoc.Content())
	}
}
