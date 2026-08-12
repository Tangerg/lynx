package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agentmemory"
)

func (a *app) ShowAgentMemory(argument string) error {
	if a.agentMemory == nil {
		return errors.New("this runtime composition has no agent memory service")
	}
	target, err := parseAgentMemoryTarget(argument, a.session.Workspace.Path)
	if err != nil {
		return err
	}
	a.showAgentMemory(target)
	return nil
}

func (a *app) showAgentMemory(target agentmemory.Target) {
	a.executeRuntimeReaderQuery(a.agentMemoryReaderQuery(target))
}

func (a *app) agentMemoryReaderQuery(target agentmemory.Target) runtimeReaderQuery {
	return runtimeReaderQuery{
		status: "loading " + string(target.Scope) + " agent memory", mode: runtimeReaderAgentMemory,
		selection: runtimeReaderSelection{agentMemoryTarget: target},
		read: func(ctx context.Context) (readerDocument, error) {
			items, err := a.agentMemory.Items(ctx, target)
			if err != nil {
				return readerDocument{}, err
			}
			return agentMemoryDocument(target, items), nil
		},
	}
}

func agentMemoryDocument(target agentmemory.Target, items []agentmemory.Item) readerDocument {
	title := "Agent memory · " + string(target.Scope)
	detail := fmt.Sprintf("%d items", len(items))
	if target.Workspace != "" {
		detail += " · " + target.Workspace
	}
	if len(items) == 0 {
		return paragraphDocument(title, detail, []string{"No active or pending memory is stored in this scope."})
	}
	sections := make([]ToolSection, 0, len(items)*2)
	for _, item := range items {
		state := string(item.Status)
		if item.Pinned {
			state += " · pinned"
		}
		metadata := []string{
			"id       " + item.ID,
			"scope    " + string(item.Scope),
			"origin   " + string(item.Origin),
			"status   " + state,
			"created  " + item.CreatedAt.Format(time.RFC3339),
			"updated  " + item.UpdatedAt.Format(time.RFC3339),
		}
		if item.SessionID != "" {
			metadata = append(metadata, "session  "+item.SessionID)
		}
		if item.Day != "" {
			metadata = append(metadata, "day      "+item.Day)
		}
		sections = append(sections,
			ToolSection{Title: state, Style: toolSectionParagraph, Text: item.Content, Links: true},
			ToolSection{Title: "Provenance", Style: toolSectionCode, Language: "text", Text: strings.Join(metadata, "\n")},
		)
	}
	return readerDocument{Title: title, Detail: detail, Sections: sections}
}

func (a *app) AddAgentMemory(argument string) error {
	if a.agentMemory == nil {
		return errors.New("this runtime composition has no agent memory service")
	}
	target, err := parseAgentMemoryTarget(argument, a.session.Workspace.Path)
	if err != nil {
		return err
	}
	a.openContextEditor(contextEditorRequest{
		Title:       "Add " + string(target.Scope) + " memory",
		Description: "User-authored memory becomes active immediately.",
		Placeholder: "Write one durable fact. Enter inserts a newline; Ctrl+S saves.",
		Save: func(content string, complete func(error) bool) error {
			content = strings.TrimSpace(content)
			if content == "" {
				return errors.New("memory content is empty")
			}
			return a.addAgentMemory(target, content, complete)
		},
	})
	return nil
}

func (a *app) EditAgentMemory(argument string) error {
	return a.loadAgentMemoryItem(argument, "loading agent memory to edit", func(target agentmemory.Target, item agentmemory.Item) {
		a.openContextEditor(contextEditorRequest{
			Title:       "Edit agent memory · " + item.ID,
			Description: "The item identity and provenance are preserved.",
			Content:     item.Content,
			Placeholder: "Memory content",
			Save: func(content string, complete func(error) bool) error {
				content = strings.TrimSpace(content)
				if content == "" {
					return errors.New("memory content is empty")
				}
				if content == item.Content {
					a.message("agent memory unchanged · " + item.ID)
					complete(nil)
					return nil
				}
				return a.updateAgentMemory(target, agentmemory.Patch{ID: item.ID, Content: &content}, "updating agent memory "+item.ID, complete)
			},
		})
	})
}

func (a *app) SetAgentMemoryPinned(argument string, pinned bool) error {
	verb := "pinning"
	if !pinned {
		verb = "unpinning"
	}
	return a.loadAgentMemoryItem(argument, verb+" agent memory", func(target agentmemory.Target, item agentmemory.Item) {
		if item.Pinned == pinned {
			state := "unpinned"
			if pinned {
				state = "pinned"
			}
			a.message("agent memory is already " + state + " · " + item.ID)
			return
		}
		if err := a.updateAgentMemory(target, agentmemory.Patch{ID: item.ID, Pinned: &pinned}, verb+" agent memory "+item.ID, nil); err != nil {
			a.message(err.Error())
		}
	})
}

func (a *app) PrepareAgentMemoryReview(argument string, approve bool) error {
	action, verb := "Reject", "rejecting"
	decision := agentmemory.Reject
	if approve {
		action, verb, decision = "Approve", "approving", agentmemory.Approve
	}
	return a.loadAgentMemoryItem(argument, verb+" agent memory", func(target agentmemory.Target, item agentmemory.Item) {
		if item.Status != agentmemory.Pending {
			a.message("only pending agent memory can be reviewed · " + item.ID)
			return
		}
		a.confirmAction(
			action+" agent memory",
			action+" pending item "+item.ID+"?",
			action,
			func() { a.reviewAgentMemory(target, item.ID, decision) },
		)
	})
}

func (a *app) PrepareDeleteAgentMemory(argument string) error {
	return a.loadAgentMemoryItem(argument, "loading agent memory to delete", func(target agentmemory.Target, item agentmemory.Item) {
		a.confirmAction("Delete agent memory", "Delete item "+item.ID+" permanently?", "Delete permanently", func() {
			a.deleteAgentMemory(target, item.ID)
		})
	})
}

func (a *app) loadAgentMemoryItem(argument, label string, apply func(agentmemory.Target, agentmemory.Item)) error {
	if a.agentMemory == nil {
		return errors.New("this runtime composition has no agent memory service")
	}
	target, identity, err := parseAgentMemoryIdentity(argument, a.session.Workspace.Path)
	if err != nil {
		return err
	}
	a.status.note(label)
	started := runOperation(a, agentMemoryOperation, false,
		func(ctx context.Context) (agentmemory.Item, error) {
			items, err := a.agentMemory.Items(ctx, target)
			if err != nil {
				return agentmemory.Item{}, err
			}
			return resolveAgentMemory(items, identity)
		},
		func(item agentmemory.Item, err error) {
			if err != nil {
				a.message(label + " failed: " + err.Error())
				return
			}
			apply(target, item)
		},
	)
	if !started {
		return errors.New("another agent memory operation is running")
	}
	return nil
}

func resolveAgentMemory(items []agentmemory.Item, identity string) (agentmemory.Item, error) {
	for _, item := range items {
		if item.ID == identity {
			return item, nil
		}
	}
	var matches []agentmemory.Item
	for _, item := range items {
		if strings.HasPrefix(item.ID, identity) {
			matches = append(matches, item)
		}
	}
	switch len(matches) {
	case 0:
		return agentmemory.Item{}, errors.New("agent memory not found: " + identity)
	case 1:
		return matches[0], nil
	default:
		return agentmemory.Item{}, errors.New("agent memory identity is ambiguous; use the full id")
	}
}

func parseAgentMemoryTarget(argument, workspace string) (agentmemory.Target, error) {
	argument = strings.TrimSpace(argument)
	if argument == "" {
		argument = string(agentmemory.Project)
	}
	scope, err := agentmemory.ParseScope(argument)
	if err != nil {
		return agentmemory.Target{}, err
	}
	if scope == agentmemory.User {
		workspace = ""
	}
	return agentmemory.NewTarget(scope, workspace)
}

func parseAgentMemoryIdentity(argument, workspace string) (agentmemory.Target, string, error) {
	fields := strings.Fields(argument)
	scope, identity := agentmemory.Project, ""
	switch len(fields) {
	case 1:
		identity = fields[0]
	case 2:
		parsed, err := agentmemory.ParseScope(fields[0])
		if err != nil {
			return agentmemory.Target{}, "", errors.New("usage: [project|user] <memory-id>")
		}
		scope, identity = parsed, fields[1]
	default:
		return agentmemory.Target{}, "", errors.New("usage: [project|user] <memory-id>")
	}
	if scope == agentmemory.User {
		workspace = ""
	}
	target, err := agentmemory.NewTarget(scope, workspace)
	return target, identity, err
}

func (a *app) addAgentMemory(target agentmemory.Target, content string, complete func(error) bool) error {
	a.status.note("adding agent memory")
	if !runOperation(a, agentMemoryOperation, false,
		func(ctx context.Context) (agentmemory.Item, error) { return a.agentMemory.Add(ctx, target, content) },
		func(item agentmemory.Item, err error) {
			if err != nil {
				a.message("add agent memory failed: " + err.Error())
				if complete != nil {
					complete(err)
				}
				return
			}
			closed := true
			if complete != nil {
				closed = complete(nil)
			}
			a.message("agent memory added · " + item.ID)
			if closed {
				a.showAgentMemory(target)
			}
		},
	) {
		return errors.New("another agent memory operation is running")
	}
	return nil
}

func (a *app) updateAgentMemory(target agentmemory.Target, patch agentmemory.Patch, label string, complete func(error) bool) error {
	a.status.note(label)
	if !runOperation(a, agentMemoryOperation, false,
		func(ctx context.Context) (agentmemory.Item, error) { return a.agentMemory.Update(ctx, patch) },
		func(item agentmemory.Item, err error) {
			if err != nil {
				a.message(label + " failed: " + err.Error())
				if complete != nil {
					complete(err)
				}
				return
			}
			closed := true
			if complete != nil {
				closed = complete(nil)
			}
			a.message("agent memory updated · " + item.ID)
			if closed {
				a.showAgentMemory(target)
			}
		},
	) {
		return errors.New("another agent memory operation is running")
	}
	return nil
}

func (a *app) reviewAgentMemory(target agentmemory.Target, id string, decision agentmemory.ReviewDecision) {
	label := string(decision) + " agent memory " + id
	a.status.note(label)
	if !runOperation(a, agentMemoryOperation, false,
		func(ctx context.Context) (string, error) { return id, a.agentMemory.Review(ctx, id, decision) },
		func(reviewed string, err error) {
			if err != nil {
				a.message(label + " failed: " + err.Error())
				return
			}
			outcome := "rejected"
			if decision == agentmemory.Approve {
				outcome = "approved"
			}
			a.message("agent memory " + outcome + " · " + reviewed)
			a.showAgentMemory(target)
		},
	) {
		a.message("another agent memory operation is running")
	}
}

func (a *app) deleteAgentMemory(target agentmemory.Target, id string) {
	a.status.note("deleting agent memory " + id)
	if !runOperation(a, agentMemoryOperation, false,
		func(ctx context.Context) (string, error) { return id, a.agentMemory.Delete(ctx, id) },
		func(deleted string, err error) {
			if err != nil {
				a.message("delete agent memory failed: " + err.Error())
				return
			}
			a.message("agent memory deleted · " + deleted)
			a.showAgentMemory(target)
		},
	) {
		a.message("another agent memory operation is running")
	}
}
