package terminal

import (
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/markdown"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func presentUser(p BlockPresentation, block agent.Block) []headless.Block {
	body := strings.TrimSpace(block.Text)
	if len(block.Attachments) > 0 {
		lines := make([]string, 0, len(block.Attachments))
		for _, item := range block.Attachments {
			lines = append(lines, "@"+item.Name+" · "+item.MimeType)
		}
		if body != "" {
			body += "\n\n"
		}
		body += strings.Join(lines, "\n")
	}
	speaker := p.Speaker
	if speaker == "" {
		speaker = "you"
	}
	return []headless.Block{newUserMessageBlockAs(p.Theme, speaker, body, speaker == "you")}
}

func presentMarkdown(speaker string) func(BlockPresentation, agent.Block) []headless.Block {
	return func(p BlockPresentation, block agent.Block) []headless.Block {
		label := speaker
		if p.Speaker != "" {
			label = p.Speaker
		}
		message := &markdownBlock{theme: p.Theme, speaker: label}
		look := p.Look
		if block.Kind == agent.BlockReasoning {
			look.Text, look.Strong = p.Theme.Muted, p.Theme.Subtle
		}
		message.doc.SetBlocks(markdown.Render(block.Text, look))
		rendered := []headless.Block{message}
		for _, image := range block.Images {
			if p.Image == nil {
				rendered = append(rendered, fallbackInlineImage(p.Theme, image))
				continue
			}
			rendered = append(rendered, p.Image(image))
		}
		return rendered
	}
}

func presentTool(p BlockPresentation, block agent.Block) []headless.Block {
	return []headless.Block{newToolBlock(p, block)}
}

func presentQuestion(p BlockPresentation, block agent.Block) []headless.Block {
	if block.Question == nil {
		return nil
	}
	lines := make([]string, 0, len(block.Question.Fields)*2)
	for index, field := range block.Question.Fields {
		lines = append(lines, p.Glyphs.Bullet+" "+field.Prompt)
		if block.Question.Answered() {
			lines = append(lines, "  answer · "+strings.Join(block.Question.Answers[index], ", "))
		}
	}
	body := strings.Join(lines, "\n")
	if block.Question.Detail != "" {
		body = block.Question.Detail + "\n" + body
	}
	return []headless.Block{&kit.Message{Theme: p.Theme, Speaker: block.Question.Title, Body: body}}
}

func presentNotice(p BlockPresentation, block agent.Block) []headless.Block {
	return []headless.Block{&kit.Message{Theme: p.Theme, Speaker: "notice", Body: block.Text}}
}

func presentFailure(p BlockPresentation, block agent.Block) []headless.Block {
	return []headless.Block{presentError(p.Theme, block.Text)}
}
