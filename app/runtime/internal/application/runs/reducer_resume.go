package runs

import (
	"encoding/json"
	"maps"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

type resumeBinding struct {
	toolItems map[string]string
	byName    map[string]string
	questions []resumedQuestion
}

type resumedQuestion struct {
	itemID   string
	question *Question
}

func resumeBindingFrom(pending interrupts.Pending) *resumeBinding {
	items := map[string]string{}
	byName := map[string]string{}
	addItem := func(name, arguments, itemID string) {
		items[resumeKey(name, arguments)] = itemID
		if _, duplicate := byName[name]; duplicate {
			byName[name] = ""
		} else {
			byName[name] = itemID
		}
	}

	var questions []resumedQuestion
	for _, in := range pending.Interrupts {
		if in.ItemID == "" {
			continue
		}
		switch in.Kind {
		case transcript.ApprovalInterrupt:
			if in.Approval != nil && in.Approval.Tool.Name != "" {
				addItem(in.Approval.Tool.Name, argsKey(in.Approval.Tool.Arguments), in.ItemID)
			}
		case transcript.QuestionInterrupt:
			questions = append(questions, resumedQuestion{itemID: in.ItemID, question: in.Question})
		}
	}
	for _, tool := range pending.DrainedTools {
		if tool.Name != "" && tool.ItemID != "" {
			addItem(tool.Name, argsKey(parseArgs(tool.Arguments)), tool.ItemID)
		}
	}
	if len(items) == 0 && len(questions) == 0 {
		return nil
	}
	return &resumeBinding{toolItems: items, byName: byName, questions: questions}
}

func resumeKey(toolName, arguments string) string { return toolName + "\x00" + arguments }

func argsKey(args map[string]any) string {
	b, _ := json.Marshal(args)
	return string(b)
}

func (r *reducer) reuseOrNextItemID(toolName, rawArguments string) string {
	if r.resume != nil {
		key := resumeKey(toolName, argsKey(parseArgs(rawArguments)))
		if id, ok := r.resume.toolItems[key]; ok {
			r.resume.consumeToolItem(id)
			return id
		}
		if id, ok := r.resume.byName[toolName]; ok && id != "" {
			r.resume.consumeToolItem(id)
			return id
		}
	}
	return r.nextItemID()
}

func (b *resumeBinding) consumeToolItem(id string) {
	maps.DeleteFunc(b.toolItems, func(_ string, candidate string) bool { return candidate == id })
	maps.DeleteFunc(b.byName, func(_ string, candidate string) bool { return candidate == id })
}

func (r *reducer) resumeQuestionCompletions() []RunEvent {
	if r.resume == nil || len(r.resume.questions) == 0 {
		return nil
	}
	out := make([]RunEvent, 0, len(r.resume.questions))
	for _, question := range r.resume.questions {
		out = append(out, ItemCompleted{Item: Item{
			ID: question.itemID, RunID: r.cfg.RunID, Status: ItemSucceeded,
			Kind: QuestionItem, CreatedAt: r.now(), Question: question.question,
		}})
	}
	r.resume.questions = nil
	return out
}
