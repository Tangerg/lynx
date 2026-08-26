package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
)

type registeredCommand struct {
	category  string
	arguments ArgumentMode
	evaluate  func(*app) CommandAvailability
}

func (r registeredCommand) availability(host *app) (availability CommandAvailability) {
	if r.evaluate == nil {
		return CommandAvailability{Enabled: true}
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			availability = CommandAvailability{Reason: fmt.Sprintf("availability check panicked: %v", recovered)}
		}
	}()
	availability = r.evaluate(host)
	availability.Reason = strings.TrimSpace(availability.Reason)
	if !availability.Enabled && availability.Reason == "" {
		availability.Reason = "not available in the current context"
	}
	return availability
}

type commandCatalog struct {
	index         headless.Commands
	registrations map[string]registeredCommand
}

func newCommandCatalog() commandCatalog {
	return commandCatalog{registrations: make(map[string]registeredCommand)}
}

func (c *commandCatalog) reset() {
	for _, found := range c.index.Find("") {
		c.index.Remove(found.Command.Name)
	}
	clear(c.registrations)
}

func (c *commandCatalog) add(owner string, descriptor CommandDescriptor, run func(string), evaluate func(*app) CommandAvailability) error {
	for _, identity := range descriptor.identities() {
		if existing, found := c.index.Lookup(identity); found {
			return fmt.Errorf("plugin %s command /%s conflicts with /%s", owner, descriptor.Name, existing.Name)
		}
	}
	c.index.Add(headless.Command{
		Name: descriptor.Name, Title: descriptor.Title, Aliases: descriptor.Aliases,
		Takes: descriptor.Arguments.TakesInput(), Run: run,
	})
	c.registrations[descriptor.Name] = registeredCommand{
		category: descriptor.category(), arguments: descriptor.Arguments, evaluate: evaluate,
	}
	return nil
}

func (c *commandCatalog) find(query string) []headless.Found {
	found := c.index.Find(query)
	exact, ok := c.index.Lookup(query)
	if !ok {
		return found
	}
	for index := range found {
		if found[index].Command.Name != exact.Name {
			continue
		}
		if index > 0 {
			exactMatch := found[index]
			copy(found[1:index+1], found[:index])
			found[0] = exactMatch
		}
		return found
	}
	return append([]headless.Found{{Command: exact}}, found...)
}

func (c *commandCatalog) lookup(identity string) (headless.Command, bool) {
	return c.index.Lookup(identity)
}

func (c *commandCatalog) used(name string) {
	c.index.Used(name)
}

func (c *commandCatalog) category(name string) string {
	return c.registrations[name].category
}

func (c *commandCatalog) arguments(name string) ArgumentMode {
	return c.registrations[name].arguments
}

func (c *commandCatalog) availability(name string, host *app) CommandAvailability {
	command, ok := c.registrations[name]
	if !ok {
		return CommandAvailability{Enabled: true}
	}
	return command.availability(host)
}

func (a *app) registerCommands() {
	a.commands.reset()
	for _, local := range builtinCommands() {
		command := local
		if err := command.validate(); err != nil {
			a.message(err.Error())
			continue
		}
		if err := a.commands.add("terminal", command.Descriptor,
			func(argument string) {
				if err := runLocalCommandSafely(command, a, argument); err != nil {
					a.message(err.Error())
				}
			},
			command.Available,
		); err != nil {
			a.message(err.Error())
		}
	}
	for _, contributed := range a.registry.OwnedValues(SlashCommands) {
		command := contributed.Value
		pluginID := contributed.PluginID
		if err := command.validate(); err != nil {
			a.message("plugin " + pluginID + ": " + err.Error())
			continue
		}
		var evaluate func(*app) CommandAvailability
		if command.Available != nil {
			evaluate = func(host *app) CommandAvailability {
				request := CommandRequest{Workspace: host.session.Workspace.Path, SessionID: host.session.ID}
				return command.Available(request)
			}
		}
		if err := a.commands.add(pluginID, command.Descriptor,
			func(argument string) { a.executeCommand(pluginID, command, argument) },
			evaluate,
		); err != nil {
			a.message(err.Error())
		}
	}
}

type commandOperation struct {
	pluginID string
	slot     operationSlot
}

func (a *app) executeCommand(pluginID string, command SlashCommand, argument string) {
	name := command.Descriptor.Name
	a.status.note("running /" + name)
	request := CommandRequest{Argument: argument, Workspace: a.session.Workspace.Path, SessionID: a.session.ID}
	dispatcher := a.loop.Dispatcher()
	a.commandSeq++
	sequence := a.commandSeq
	slot := operationSlot(fmt.Sprintf("command:%d", sequence))
	a.commandOperations[sequence] = commandOperation{pluginID: pluginID, slot: slot}
	started := a.operations.GoSession(slot, false, func(ctx context.Context, lease operationLease) {
		result, err := executeCommandSafely(ctx, command, request)
		_ = post(ctx, dispatcher, func() {
			if !a.operations.Current(lease) || a.closed {
				return
			}
			delete(a.commandOperations, sequence)
			if errors.Is(err, context.Canceled) {
				return
			}
			if err != nil {
				a.message(err.Error())
				return
			}
			message := strings.TrimSpace(result.Message)
			if message == "" {
				message = "completed /" + name
			}
			a.message(message)
		})
	})
	if !started {
		delete(a.commandOperations, sequence)
		a.message("could not start /" + name)
	}
}

func (a *app) cancelPluginCommands(pluginIDs ...string) {
	selected := make(map[string]struct{}, len(pluginIDs))
	for _, pluginID := range pluginIDs {
		selected[pluginID] = struct{}{}
	}
	for sequence, operation := range a.commandOperations {
		if len(selected) > 0 {
			if _, cancel := selected[operation.pluginID]; !cancel {
				continue
			}
		}
		a.operations.Cancel(operation.slot)
		delete(a.commandOperations, sequence)
	}
}

func executeCommandSafely(ctx context.Context, command SlashCommand, request CommandRequest) (result CommandResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("command /%s panicked: %v", command.Descriptor.Name, recovered)
		}
	}()
	return command.Execute(ctx, request)
}

func runLocalCommandSafely(command localCommand, host *app, argument string) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("command /%s panicked: %v", command.Descriptor.Name, recovered)
		}
	}()
	return command.Run(host, argument)
}

func (a *app) runCommand(name, argument string) {
	command, ok := a.commands.lookup(name)
	if !ok || command.Run == nil {
		a.message("unknown command: /" + name)
		return
	}
	if availability := a.commands.availability(command.Name, a); !availability.Enabled {
		a.message("/" + command.Name + " unavailable: " + availability.Reason)
		return
	}
	argument = strings.TrimSpace(argument)
	if err := a.commands.arguments(command.Name).ValidateInvocation(command.Name, argument); err != nil {
		a.message(err.Error())
		return
	}
	a.commands.used(command.Name)
	command.Run(argument)
}
