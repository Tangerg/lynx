package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/headless"

	"github.com/Tangerg/lynx/app/cli/internal/extensions"
)

type registeredCommand struct {
	category string
	evaluate func(*app) CommandAvailability
}

func (command registeredCommand) availability(host *app) (availability CommandAvailability) {
	if command.evaluate == nil {
		return CommandAvailability{Enabled: true}
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			availability = CommandAvailability{Reason: fmt.Sprintf("availability check panicked: %v", recovered)}
		}
	}()
	availability = command.evaluate(host)
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

func (catalog *commandCatalog) reset() {
	for _, found := range catalog.index.Find("") {
		catalog.index.Remove(found.Command.Name)
	}
	clear(catalog.registrations)
}

func (catalog *commandCatalog) add(owner string, descriptor CommandDescriptor, run func(string), evaluate func(*app) CommandAvailability) error {
	for _, identity := range descriptor.identities() {
		if existing, found := catalog.index.Lookup(identity); found {
			return fmt.Errorf("plugin %s command /%s conflicts with /%s", owner, descriptor.Name, existing.Name)
		}
	}
	catalog.index.Add(headless.Command{
		Name: descriptor.Name, Title: descriptor.Title, Aliases: descriptor.Aliases, Takes: descriptor.Takes, Run: run,
	})
	catalog.registrations[descriptor.Name] = registeredCommand{category: descriptor.category(), evaluate: evaluate}
	return nil
}

func (catalog *commandCatalog) find(query string) []headless.Found {
	found := catalog.index.Find(query)
	exact, ok := catalog.index.Lookup(query)
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

func (catalog *commandCatalog) lookup(identity string) (headless.Command, bool) {
	return catalog.index.Lookup(identity)
}

func (catalog *commandCatalog) used(name string) {
	catalog.index.Used(name)
}

func (catalog *commandCatalog) category(name string) string {
	return catalog.registrations[name].category
}

func (catalog *commandCatalog) availability(name string, host *app) CommandAvailability {
	command, ok := catalog.registrations[name]
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
	for _, contributed := range extensions.OwnedValues(a.registry, SlashCommands) {
		command := contributed.Value
		pluginID := contributed.PluginID
		if err := command.validate(); err != nil {
			a.message("plugin " + pluginID + ": " + err.Error())
			continue
		}
		var evaluate func(*app) CommandAvailability
		if command.Available != nil {
			evaluate = func(host *app) CommandAvailability {
				request := CommandRequest{Workspace: host.session.Workspace, SessionID: host.session.ID}
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
	request := CommandRequest{Argument: argument, Workspace: a.session.Workspace, SessionID: a.session.ID}
	dispatcher := a.loop.Dispatcher()
	a.commandSeq++
	sequence := a.commandSeq
	slot := operationSlot(fmt.Sprintf("command:%d", sequence))
	a.commandOperations[sequence] = commandOperation{pluginID: pluginID, slot: slot}
	started := a.operations.Go(slot, false, func(ctx context.Context, lease operationLease) {
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
	if command.Takes && strings.TrimSpace(argument) == "" {
		a.message("/" + command.Name + " needs an argument")
		return
	}
	a.commands.used(command.Name)
	command.Run(strings.TrimSpace(argument))
}
