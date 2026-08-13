package terminal

import (
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

// questionBlock separates an open human decision from its durable transcript
// fact. A pending Question occupies its stable transcript position without
// drawing an interaction-looking prompt before the dialog owns input. Once the
// runtime accepts the answer, the same block becomes visible in place.
type questionBlock struct {
	theme    kit.Theme
	glyphs   kit.Glyphs
	question agent.Question
	message  kit.Message
}

var (
	_ headless.Block    = (*questionBlock)(nil)
	_ headless.Copyable = (*questionBlock)(nil)
)

func newQuestionBlock(theme kit.Theme, glyphs kit.Glyphs, question agent.Question) *questionBlock {
	block := &questionBlock{theme: theme, glyphs: glyphs}
	block.setQuestion(question)
	return block
}

func (b *questionBlock) answered() bool {
	return b != nil && b.question.Answered()
}

func (b *questionBlock) validateAccepted(question agent.Question) error {
	if b == nil {
		return fmt.Errorf("question presentation is absent")
	}
	if !question.Answered() {
		return fmt.Errorf("question %s has no accepted answers", question.ItemID)
	}
	expected, err := b.question.Accept(agent.QuestionAnswer{Values: question.Answers})
	if err != nil {
		return err
	}
	if !expected.Equal(question) {
		return fmt.Errorf("accepted question %s differs from its pending transcript item", question.ItemID)
	}
	return nil
}

func (b *questionBlock) accept(question agent.Question) {
	b.setQuestion(question)
}

func (b *questionBlock) Measure(width int) int {
	if !b.answered() {
		return 0
	}
	return b.message.Measure(width)
}

func (b *questionBlock) Draw(view grid.View) {
	if b.answered() {
		b.message.Draw(view)
	}
}

func (b *questionBlock) Rows(width int) []text.Row {
	if !b.answered() {
		return nil
	}
	return b.message.Rows(width)
}

func (b *questionBlock) setQuestion(question agent.Question) {
	b.question = question.Clone()
	b.message = kit.Message{
		Theme: b.theme, Speaker: question.Title,
		Body: presentQuestionBody(b.glyphs, question),
	}
}

func presentQuestionBody(glyphs kit.Glyphs, question agent.Question) string {
	lines := make([]string, 0, len(question.Fields)*2)
	for index, field := range question.Fields {
		lines = append(lines, glyphs.Bullet+" "+field.Prompt)
		if question.Answered() {
			lines = append(lines, "  answer · "+strings.Join(question.Answers[index], ", "))
		}
	}
	body := strings.Join(lines, "\n")
	if question.Detail != "" {
		body = question.Detail + "\n" + body
	}
	return body
}
