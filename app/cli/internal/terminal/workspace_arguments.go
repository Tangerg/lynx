package terminal

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/workspace"
)

type commandOptionCursor struct {
	remaining string
	seen      map[string]struct{}
	stopped   bool
}

func newCommandOptionCursor(argument string) *commandOptionCursor {
	return &commandOptionCursor{remaining: strings.TrimSpace(argument), seen: make(map[string]struct{})}
}

func (c *commandOptionCursor) Next() (string, bool, error) {
	if c.stopped || c.remaining == "" {
		return "", false, nil
	}
	token, rest := nextCommandToken(c.remaining)
	if token == "--" {
		c.remaining, c.stopped = strings.TrimSpace(rest), true
		return "", false, nil
	}
	if !strings.HasPrefix(token, "--") {
		return "", false, nil
	}
	c.remaining = strings.TrimSpace(rest)
	if _, duplicate := c.seen[token]; duplicate {
		return "", false, fmt.Errorf("option %s was specified more than once", token)
	}
	c.seen[token] = struct{}{}
	return token, true, nil
}

func (c *commandOptionCursor) Value(option string) (string, error) {
	value, rest := nextCommandToken(c.remaining)
	if value == "" || strings.HasPrefix(value, "--") {
		return "", fmt.Errorf("option %s requires a value", option)
	}
	c.remaining = strings.TrimSpace(rest)
	return value, nil
}

func (c *commandOptionCursor) PositiveInt(option string) (int, error) {
	value, err := c.Value(option)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("option %s requires a positive integer", option)
	}
	return parsed, nil
}

func (c *commandOptionCursor) Rest() string { return strings.TrimSpace(c.remaining) }

func nextCommandToken(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	if boundary := strings.IndexAny(value, " \t\r\n"); boundary >= 0 {
		return value[:boundary], value[boundary:]
	}
	return value, ""
}

type workspaceDiffSelection struct {
	path   string
	mode   workspace.DiffMode
	format workspace.DiffFormat
	limit  int
}

func parseWorkspaceDiffSelection(argument string) (workspaceDiffSelection, error) {
	selection := workspaceDiffSelection{mode: workspace.DiffModeWorktree, format: workspace.DiffFormatRaw}
	cursor := newCommandOptionCursor(argument)
	var modeSet, formatSet bool
	for {
		option, ok, err := cursor.Next()
		if err != nil {
			return workspaceDiffSelection{}, err
		}
		if !ok {
			break
		}
		switch option {
		case "--base", "--worktree":
			if modeSet {
				return workspaceDiffSelection{}, errors.New("workspace diff mode was specified more than once")
			}
			modeSet = true
			if option == "--base" {
				selection.mode = workspace.DiffModeBase
			}
		case "--rows", "--raw":
			if formatSet {
				return workspaceDiffSelection{}, errors.New("workspace diff format was specified more than once")
			}
			formatSet = true
			if option == "--rows" {
				selection.format = workspace.DiffFormatRows
			}
		case "--limit":
			selection.limit, err = cursor.PositiveInt(option)
			if err != nil {
				return workspaceDiffSelection{}, err
			}
		default:
			return workspaceDiffSelection{}, fmt.Errorf("unknown workspace diff option %q", option)
		}
	}
	selection.path = cursor.Rest()
	if selection.limit > 0 && selection.format != workspace.DiffFormatRows {
		return workspaceDiffSelection{}, errors.New("workspace diff --limit requires --rows")
	}
	return selection, nil
}

type workspaceHeadSelection struct {
	path  string
	lines int
}

func parseWorkspaceHeadSelection(argument string) (workspaceHeadSelection, error) {
	selection := workspaceHeadSelection{lines: 80}
	cursor := newCommandOptionCursor(argument)
	for {
		option, ok, err := cursor.Next()
		if err != nil {
			return workspaceHeadSelection{}, err
		}
		if !ok {
			break
		}
		if option != "--lines" {
			return workspaceHeadSelection{}, fmt.Errorf("unknown file preview option %q", option)
		}
		selection.lines, err = cursor.PositiveInt(option)
		if err != nil {
			return workspaceHeadSelection{}, err
		}
	}
	selection.path = cursor.Rest()
	if selection.path == "" {
		return workspaceHeadSelection{}, errors.New("usage: /preview [--lines N] <path>")
	}
	return selection, nil
}

type workspaceSearchSelection struct {
	query string
	path  string
	limit int
}

func parseWorkspaceSearchSelection(argument string) (workspaceSearchSelection, error) {
	selection := workspaceSearchSelection{limit: 200}
	cursor := newCommandOptionCursor(argument)
	for {
		option, ok, err := cursor.Next()
		if err != nil {
			return workspaceSearchSelection{}, err
		}
		if !ok {
			break
		}
		switch option {
		case "--path":
			selection.path, err = cursor.Value(option)
		case "--limit":
			selection.limit, err = cursor.PositiveInt(option)
		default:
			return workspaceSearchSelection{}, fmt.Errorf("unknown workspace search option %q", option)
		}
		if err != nil {
			return workspaceSearchSelection{}, err
		}
	}
	selection.query = cursor.Rest()
	if selection.query == "" {
		return workspaceSearchSelection{}, errors.New("usage: /grep [--path PATH] [--limit N] <query>")
	}
	return selection, nil
}

type workspaceFilesSelection struct {
	path           string
	glob           string
	recursive      bool
	includeIgnored bool
}

func parseWorkspaceFilesSelection(argument string) (workspaceFilesSelection, error) {
	selection := workspaceFilesSelection{}
	cursor := newCommandOptionCursor(argument)
	for {
		option, ok, err := cursor.Next()
		if err != nil {
			return workspaceFilesSelection{}, err
		}
		if !ok {
			break
		}
		switch option {
		case "--recursive":
			selection.recursive = true
		case "--ignored":
			selection.includeIgnored = true
		case "--glob":
			selection.glob, err = cursor.Value(option)
			if err != nil {
				return workspaceFilesSelection{}, err
			}
		default:
			return workspaceFilesSelection{}, fmt.Errorf("unknown workspace browse option %q", option)
		}
	}
	selection.path = cursor.Rest()
	return selection, nil
}

type workspaceReadSelection struct {
	path      string
	startLine int
	endLine   int
	maxBytes  int
}

func parseWorkspaceReadSelection(argument string) (workspaceReadSelection, error) {
	selection := workspaceReadSelection{maxBytes: 2 << 20}
	cursor := newCommandOptionCursor(argument)
	for {
		option, ok, err := cursor.Next()
		if err != nil {
			return workspaceReadSelection{}, err
		}
		if !ok {
			break
		}
		switch option {
		case "--start":
			selection.startLine, err = cursor.PositiveInt(option)
		case "--end":
			selection.endLine, err = cursor.PositiveInt(option)
		case "--max-bytes":
			selection.maxBytes, err = cursor.PositiveInt(option)
		default:
			return workspaceReadSelection{}, fmt.Errorf("unknown workspace read option %q", option)
		}
		if err != nil {
			return workspaceReadSelection{}, err
		}
	}
	selection.path = cursor.Rest()
	switch {
	case selection.path == "":
		return workspaceReadSelection{}, errors.New("usage: /read [--start N] [--end N] [--max-bytes N] <path>")
	case selection.endLine > 0 && selection.startLine == 0:
		return workspaceReadSelection{}, errors.New("workspace read --end requires --start")
	case selection.endLine > 0 && selection.endLine < selection.startLine:
		return workspaceReadSelection{}, errors.New("workspace read --end precedes --start")
	}
	return selection, nil
}
