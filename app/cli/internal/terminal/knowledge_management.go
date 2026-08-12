package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/knowledge"
)

func (a *app) ShowKnowledge() {
	if a.knowledge == nil {
		a.message("this runtime composition has no knowledge service")
		return
	}
	a.executeRuntimeReaderQuery(a.knowledgeEntriesReaderQuery())
}

func (a *app) knowledgeEntriesReaderQuery() runtimeReaderQuery {
	workspace := a.session.Workspace.Path
	return runtimeReaderQuery{
		status: "loading LYRA.md knowledge", mode: runtimeReaderKnowledge,
		read: func(ctx context.Context) (readerDocument, error) {
			entries, err := a.knowledge.Entries(ctx, workspace)
			if err != nil {
				return readerDocument{}, err
			}
			return knowledgeEntriesDocument(workspace, entries), nil
		},
	}
}

func knowledgeEntriesDocument(workspace string, entries []knowledge.Entry) readerDocument {
	if len(entries) == 0 {
		return paragraphDocument("LYRA.md knowledge", workspace, []string{"No knowledge documents exist in the cascade."})
	}
	sections := make([]ToolSection, 0, len(entries))
	for _, entry := range entries {
		detail := string(entry.Scope)
		if entry.UpdatedAt != nil {
			detail += " · updated " + entry.UpdatedAt.Format(time.RFC3339)
		}
		content := entry.Content
		if content == "" {
			content = "(empty document)"
		}
		sections = append(sections, ToolSection{Title: detail, Style: toolSectionParagraph, Text: content, Links: true})
	}
	return readerDocument{
		Title: "LYRA.md knowledge", Detail: fmt.Sprintf("%d cascade entries · %s", len(entries), workspace), Sections: sections,
	}
}

func (a *app) ReadKnowledge(argument string) error {
	if a.knowledge == nil {
		return errors.New("this runtime composition has no knowledge service")
	}
	target, err := parseKnowledgeTarget(argument, a.session.Workspace.Path)
	if err != nil {
		return err
	}
	a.readKnowledge(target)
	return nil
}

func (a *app) readKnowledge(target knowledge.Target) {
	a.executeRuntimeReaderQuery(a.knowledgeDocumentReaderQuery(target))
}

func (a *app) knowledgeDocumentReaderQuery(target knowledge.Target) runtimeReaderQuery {
	return runtimeReaderQuery{
		status: "loading " + string(target.Scope) + " LYRA.md", mode: runtimeReaderKnowledge,
		selection: runtimeReaderSelection{knowledgeTarget: target, knowledgeEntry: true},
		read: func(ctx context.Context) (readerDocument, error) {
			entry, err := a.knowledge.Document(ctx, target)
			if err != nil {
				return readerDocument{}, err
			}
			return knowledgeDocument(target, entry), nil
		},
	}
}

func knowledgeDocument(target knowledge.Target, entry knowledge.Entry) readerDocument {
	detail := string(target.Scope)
	if target.Workspace != "" {
		detail += " · " + target.Workspace
	}
	if entry.UpdatedAt != nil {
		detail += " · updated " + entry.UpdatedAt.Format(time.RFC3339)
	}
	content := entry.Content
	if content == "" {
		content = "(empty document)"
	}
	return readerDocument{
		Title: "LYRA.md · " + string(target.Scope), Detail: detail,
		Sections: []ToolSection{{Title: "Content", Style: toolSectionParagraph, Text: content, Links: true}},
	}
}

func (a *app) EditKnowledge(argument string) error {
	if a.knowledge == nil {
		return errors.New("this runtime composition has no knowledge service")
	}
	target, err := parseKnowledgeTarget(argument, a.session.Workspace.Path)
	if err != nil {
		return err
	}
	a.status.note("loading " + string(target.Scope) + " LYRA.md to edit")
	if !runOperation(a, knowledgeOperation, false,
		func(ctx context.Context) (knowledge.Entry, error) { return a.knowledge.Document(ctx, target) },
		func(entry knowledge.Entry, err error) {
			if err != nil {
				a.message("load LYRA.md to edit failed: " + err.Error())
				return
			}
			current := entry
			a.openContextEditor(
				"Edit LYRA.md · "+string(target.Scope),
				"Enter inserts a newline. Ctrl+S saves; an empty document clears this scope.",
				entry.Content, "Human-authored instructions and project knowledge",
				func(content string, complete func(error) bool) error {
					if content == entry.Content {
						a.message("LYRA.md unchanged · " + string(target.Scope))
						complete(nil)
						return nil
					}
					return a.saveKnowledge(&current, target, content, complete)
				},
			)
		},
	) {
		return errors.New("another knowledge operation is running")
	}
	return nil
}

func (a *app) saveKnowledge(current *knowledge.Entry, target knowledge.Target, content string, complete func(error) bool) error {
	if current == nil {
		return errors.New("knowledge editor has no revision owner")
	}
	update, err := current.Revise(target, content)
	if err != nil {
		return err
	}
	a.status.note("saving " + string(target.Scope) + " LYRA.md")
	if !runOperation(a, knowledgeOperation, false,
		func(ctx context.Context) (knowledge.Entry, error) { return a.knowledge.Save(ctx, update) },
		func(saved knowledge.Entry, err error) {
			if err != nil {
				a.message("save LYRA.md failed: " + err.Error())
				if complete != nil {
					complete(err)
				}
				return
			}
			*current = saved
			closed := true
			if complete != nil {
				closed = complete(nil)
			}
			a.message("LYRA.md saved · " + string(target.Scope))
			if closed {
				a.openReaderDocument(knowledgeDocument(target, saved))
			}
		},
	) {
		return errors.New("another knowledge operation is running")
	}
	return nil
}

func parseKnowledgeTarget(argument, workspace string) (knowledge.Target, error) {
	scope, err := knowledge.ParseScope(strings.TrimSpace(argument))
	if err != nil {
		return knowledge.Target{}, errors.New("usage: <cwd|projectRoot|home>")
	}
	if scope == knowledge.Home {
		workspace = ""
	}
	return knowledge.NewTarget(scope, workspace)
}
