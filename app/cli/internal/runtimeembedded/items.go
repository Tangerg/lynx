package runtimeembedded

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func projectItem(value protocol.Item) (agent.Block, error) {
	status := agent.BlockStatus(value.Status)
	block := agent.Block{ID: value.ID, RunID: value.RunID, Status: status}
	switch value.Type {
	case protocol.ItemTypeUserMessage:
		block.Kind = agent.BlockUser
		text, attachments, err := projectContent(value.ID, value.Content)
		if err != nil {
			return agent.Block{}, err
		}
		block.Text, block.Attachments = text, attachments
	case protocol.ItemTypeAgentMessage:
		block.Kind = agent.BlockAssistant
		text, attachments, err := projectContent(value.ID, value.Content)
		if err != nil {
			return agent.Block{}, err
		}
		if len(attachments) != 0 {
			return agent.Block{}, fmt.Errorf("agent message %s contains unsupported image output", value.ID)
		}
		block.Text = text
	case protocol.ItemTypeReasoning:
		block.Kind = agent.BlockReasoning
		block.Text = value.Text
	case protocol.ItemTypeQuestion:
		block.Kind = agent.BlockQuestion
		question, err := projectQuestion(value.ID, value.Question)
		if err != nil {
			return agent.Block{}, err
		}
		block.Question = &question
	case protocol.ItemTypeToolCall:
		block.Kind = agent.BlockTool
		tool, err := projectTool(value.Tool, value.Status, value.DurationMillis, value.Error)
		if err != nil {
			return agent.Block{}, fmt.Errorf("item %s: %w", value.ID, err)
		}
		block.Tool = &tool
	case protocol.ItemTypeCompaction:
		block.Kind = agent.BlockNotice
		block.Text = value.Summary
		if strings.TrimSpace(block.Text) == "" {
			block.Text = fmt.Sprintf("Conversation compacted; %d messages removed.", value.DroppedMessages)
		}
	default:
		return agent.Block{}, fmt.Errorf("item %s has unsupported type %q", value.ID, value.Type)
	}
	if err := validateProjectedBlock(block); err != nil {
		return agent.Block{}, fmt.Errorf("runtime item %s: %w", value.ID, err)
	}
	return block, nil
}

func validateProjectedBlock(block agent.Block) error {
	event := agent.Event(agent.BlockCompleted{Block: block})
	if block.Status == agent.BlockStatusRunning {
		event = agent.BlockStarted{Block: block}
	}
	return agent.ValidateEvent(event)
}

func projectQuestion(itemID string, value *protocol.Question) (agent.Question, error) {
	if value == nil {
		return agent.Question{}, fmt.Errorf("question item %s has no payload", itemID)
	}
	question := agent.Question{ItemID: itemID, Fields: make([]agent.QuestionField, 0, len(value.Fields))}
	for _, field := range value.Fields {
		projected := agent.QuestionField{
			Prompt: field.Prompt, Header: field.Header, AllowCustom: field.AllowCustom,
			Options: make([]agent.QuestionOption, 0, len(field.Options)),
		}
		switch field.Type {
		case protocol.QuestionFieldText:
			projected.Kind = agent.QuestionText
		case protocol.QuestionFieldChoice:
			if field.Multiple {
				projected.Kind = agent.QuestionMulti
			} else {
				projected.Kind = agent.QuestionSingle
			}
		default:
			return agent.Question{}, fmt.Errorf("question item %s has unsupported field type %q", itemID, field.Type)
		}
		for _, option := range field.Options {
			projected.Options = append(projected.Options, agent.QuestionOption{
				Label: option.Label, Description: option.Description, Preview: option.Preview,
			})
		}
		question.Fields = append(question.Fields, projected)
	}
	for _, field := range question.Fields {
		if strings.TrimSpace(field.Header) != "" {
			question.Title = field.Header
			break
		}
	}
	if question.Title == "" {
		question.Title = "Question"
	}
	if err := question.Validate(); err != nil {
		return agent.Question{}, err
	}
	return question, nil
}

func projectTool(value *protocol.ToolInvocation, status protocol.ItemStatus, durationMillis *int64, problem *protocol.ProblemData) (agent.ToolCall, error) {
	if value == nil {
		return agent.ToolCall{}, fmt.Errorf("tool payload is absent")
	}
	tool := agent.ToolCall{
		Kind: kindForTool(value.Name), Name: value.Name, Summary: toolSummary(value),
		Command: stringArgument(value.Arguments, "command"),
		Path:    firstStringArgument(value.Arguments, "path", "file", "filename"),
		Query:   firstStringArgument(value.Arguments, "query", "pattern", "search"),
		URL:     firstStringArgument(value.Arguments, "url", "uri"),
	}
	if durationMillis != nil {
		tool.Duration = time.Duration(*durationMillis) * time.Millisecond
	}
	switch status {
	case protocol.ItemStatusRunning:
		tool.Status = agent.ToolRunning
	case protocol.ItemStatusCompleted:
		tool.Status = agent.ToolOK
	case protocol.ItemStatusIncomplete:
		tool.Status = agent.ToolError
		if problem != nil && (problem.Type == protocol.ProblemDeniedByUser ||
			problem.Type == protocol.ProblemChildRunCanceled || problem.Type == protocol.ProblemToolCanceled) {
			tool.Status = agent.ToolCanceled
		}
	default:
		return agent.ToolCall{}, fmt.Errorf("tool status %q is unsupported", status)
	}
	projectToolResult(&tool, value.Result)
	if problem != nil && strings.TrimSpace(problem.Detail) != "" {
		tool.Output = problem.Detail
	}
	if err := tool.Validate(); err != nil {
		return agent.ToolCall{}, err
	}
	return tool, nil
}

func kindForTool(name string) agent.ToolKind {
	switch name {
	case "shell", "read_shell_output", "stop_shell":
		return agent.ToolShell
	case "apply_patch", "write_file", "edit_file":
		return agent.ToolEdit
	case "read", "read_file", "read_skill_resource", "read_tool_result":
		return agent.ToolRead
	case "glob", "grep", "search_memory", "search_conversations", "search_tools", "lsp":
		return agent.ToolSearch
	case "web_search", "web_fetch", "http_request":
		return agent.ToolWeb
	case "ask_user", "delegate_task", "create_goal", "get_goal", "report_goal_outcome",
		"create_schedule", "list_schedules", "delete_schedule", "load_skill", "list_skills",
		"propose_skill", "enter_plan_mode", "exit_plan_mode", "set_plan":
		return agent.ToolTask
	default:
		return agent.ToolUnknown
	}
}

func toolSummary(value *protocol.ToolInvocation) string {
	for _, key := range []string{"description", "summary", "query", "pattern", "path", "url", "command"} {
		if text := stringArgument(value.Arguments, key); text != "" {
			return truncateRunes(text, 120)
		}
	}
	return truncateRunes(value.Name, 120)
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit-1]) + "…"
}

func stringArgument(arguments map[string]any, key string) string {
	value, ok := arguments[key].(string)
	if !ok {
		return ""
	}
	return value
}

func firstStringArgument(arguments map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringArgument(arguments, key); value != "" {
			return value
		}
	}
	return ""
}

func projectToolResult(tool *agent.ToolCall, result any) {
	if result == nil {
		return
	}
	object, objectOK := result.(map[string]any)
	if objectOK {
		if output, ok := object["output"].(string); ok {
			tool.Output = output
		}
		if exitCode, ok := integerValue(object["exitCode"]); ok {
			tool.ExitCode = &exitCode
		}
		if changes, ok := object["changes"].([]any); ok {
			paths := make([]string, 0, len(changes))
			for _, change := range changes {
				if entry, ok := change.(map[string]any); ok {
					if path, ok := entry["path"].(string); ok {
						paths = append(paths, filepath.ToSlash(path))
					}
				}
			}
			if tool.Path == "" && len(paths) != 0 {
				tool.Path = paths[0]
			}
			if tool.Output == "" && len(paths) != 0 {
				tool.Output = strings.Join(paths, "\n")
			}
		}
	}
	if tool.Output == "" {
		if encoded, err := json.MarshalIndent(result, "", "  "); err == nil {
			tool.Output = string(encoded)
		}
	}
}

func integerValue(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int64:
		return int(number), int64(int(number)) == number
	case float64:
		converted := int(number)
		return converted, float64(converted) == number
	case json.Number:
		parsed, err := number.Int64()
		return int(parsed), err == nil && int64(int(parsed)) == parsed
	default:
		return 0, false
	}
}
