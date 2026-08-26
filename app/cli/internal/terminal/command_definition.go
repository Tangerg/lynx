package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

func splitCommandArgument(value string) (identity, remainder string, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", false
	}
	boundary := strings.IndexFunc(value, unicode.IsSpace)
	if boundary < 0 {
		return value, "", true
	}
	return value[:boundary], strings.TrimSpace(value[boundary:]), true
}

func trimCommandIdentity(value, identity string) (string, bool) {
	if !strings.HasPrefix(value, identity) {
		return "", false
	}
	remainder := value[len(identity):]
	if remainder == "" {
		return "", true
	}
	if boundary, _ := utf8.DecodeRuneInString(remainder); !unicode.IsSpace(boundary) {
		return "", false
	}
	return strings.TrimSpace(remainder), true
}

// SlashCommand is a contributed composer command. Extensions receive a
// bounded request snapshot rather than the terminal application itself.
type SlashCommand struct {
	Descriptor CommandDescriptor
	Available  func(CommandRequest) CommandAvailability
	Execute    func(context.Context, CommandRequest) (CommandResult, error)
}

type localCommand struct {
	Descriptor CommandDescriptor
	Available  func(*app) CommandAvailability
	Run        func(*app, string) error
}

// ArgumentMode is the cardinality of one slash command's trailing input. It is
// deliberately richer than the presenter's Takes flag: optional input must be
// discoverable without being mistaken for required input, and argument-free
// commands must reject accidental trailing text.
type ArgumentMode string

const (
	NoArguments       ArgumentMode = ""
	OptionalArguments ArgumentMode = "optional"
	RequiredArguments ArgumentMode = "required"
)

func (a ArgumentMode) Validate() error {
	if a != NoArguments && a != OptionalArguments && a != RequiredArguments {
		return fmt.Errorf("slash command argument mode %q is invalid", a)
	}
	return nil
}

func (a ArgumentMode) TakesInput() bool { return a != NoArguments }

func (a ArgumentMode) ValidateInvocation(name, argument string) error {
	argument = strings.TrimSpace(argument)
	switch a {
	case NoArguments:
		if argument != "" {
			return fmt.Errorf("/%s does not accept arguments", name)
		}
	case OptionalArguments:
	case RequiredArguments:
		if argument == "" {
			return fmt.Errorf("/%s needs an argument", name)
		}
	default:
		return fmt.Errorf("/%s has an invalid argument contract", name)
	}
	return nil
}

// CommandDescriptor defines a slash command's stable identity, presentation,
// and argument contract independently of its execution environment.
type CommandDescriptor struct {
	Name      string
	Title     string
	Category  string
	Aliases   []string
	Arguments ArgumentMode
}

// Validate checks the command identity and its aliases as one namespace.
func (c CommandDescriptor) Validate() error {
	switch {
	case strings.TrimSpace(c.Name) == "":
		return errors.New("slash command has no name")
	case strings.ContainsAny(c.Name, " /\t\n"):
		return fmt.Errorf("slash command %q has an invalid name", c.Name)
	case strings.TrimSpace(c.Title) == "":
		return fmt.Errorf("slash command %q has no title", c.Name)
	default:
		if err := c.Arguments.Validate(); err != nil {
			return fmt.Errorf("slash command %q: %w", c.Name, err)
		}
		return c.validateAliases()
	}
}

func (c CommandDescriptor) validateAliases() error {
	seen := map[string]struct{}{c.Name: {}}
	for _, alias := range c.Aliases {
		if strings.TrimSpace(alias) == "" || strings.ContainsAny(alias, " /\t\n") {
			return fmt.Errorf("slash command %q has invalid alias %q", c.Name, alias)
		}
		if _, duplicate := seen[alias]; duplicate {
			return fmt.Errorf("slash command %q repeats name or alias %q", c.Name, alias)
		}
		seen[alias] = struct{}{}
	}
	return nil
}

func (c CommandDescriptor) category() string {
	if category := strings.TrimSpace(c.Category); category != "" {
		return category
	}
	return "General"
}

func (c CommandDescriptor) identities() []string {
	return append([]string{c.Name}, c.Aliases...)
}

// CommandAvailability is the shared discovery and execution gate for a slash
// command. Disabled commands stay visible with their reason.
type CommandAvailability struct {
	Enabled bool
	Reason  string
}

// CommandRequest is the bounded product context given to an out-of-process
// slash command.
type CommandRequest struct {
	Argument  string
	Workspace string
	SessionID string
}

// CommandResult is what an asynchronous slash command may surface in the
// terminal chrome.
type CommandResult struct {
	Message string
}

func (s SlashCommand) validate() error {
	if err := s.Descriptor.Validate(); err != nil {
		return err
	}
	if s.Execute == nil {
		return fmt.Errorf("slash command %q has no handler", s.Descriptor.Name)
	}
	return nil
}

func (l localCommand) validate() error {
	if err := l.Descriptor.Validate(); err != nil {
		return err
	}
	if l.Run == nil {
		return fmt.Errorf("slash command %q has no handler", l.Descriptor.Name)
	}
	return nil
}
