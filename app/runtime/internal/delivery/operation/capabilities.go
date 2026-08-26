package operation

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/contractshape"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func (e *Endpoint) enforceCapabilities(ctx context.Context, meta MethodMeta, parameters any) *Failure {
	for _, rule := range meta.CapabilityRules {
		if len(rule.When) != 0 && !matchesAll(rule.When, reflect.ValueOf(parameters)) {
			continue
		}
		missing, err := e.missingFeatureRequirements(ctx, rule.Requires)
		if err != nil {
			return ProjectError(err)
		}
		if len(missing) != 0 {
			return ProjectError(NewCapabilityGapError(missing...))
		}
	}
	return nil
}

func (e *Endpoint) missingFeatureRequirements(
	ctx context.Context,
	required []string,
) ([]protocol.CapabilityRequirement, error) {
	discoverer, ok := e.target.(interface {
		Discover(context.Context) (*protocol.DiscoverResponse, error)
	})
	if !ok || !capabilityAvailable(discoverer) {
		return nil, errors.New("operation: target cannot handle runtime.discover")
	}
	discovered, err := discoverer.Discover(ctx)
	if err != nil {
		return nil, fmt.Errorf("operation: read capabilities: %w", err)
	}
	if discovered == nil {
		return nil, errors.New("operation: the runtime reported no capabilities")
	}
	client, _ := ClientCapabilitiesFrom(ctx)
	return protocol.MissingFeatureRequirements(
		discovered.Capabilities.Features,
		client,
		required...,
	), nil
}

func matchesAll(conditions []FieldCondition, parameters reflect.Value) bool {
	for _, condition := range conditions {
		if !condition.matches(parameters) {
			return false
		}
	}
	return true
}

func (f FieldCondition) matches(parameters reflect.Value) bool {
	value, found := lookupValue(parameters, f.Field)
	switch f.Operator {
	case OperatorPresent:
		return found && !isEmptyValue(value)
	case OperatorEquals:
		return found && value.Kind() == reflect.String && value.String() == f.Value
	default:
		return false
	}
}

func lookupValue(value reflect.Value, path string) (reflect.Value, bool) {
	current := value
	for segment := range strings.SplitSeq(path, ".") {
		for current.IsValid() && (current.Kind() == reflect.Interface || current.Kind() == reflect.Pointer) {
			if current.IsNil() {
				return reflect.Value{}, false
			}
			current = current.Elem()
		}
		if !current.IsValid() || current.Kind() != reflect.Struct {
			return reflect.Value{}, false
		}
		field, ok := contractshape.LookupField(current.Type(), segment)
		if !ok {
			return reflect.Value{}, false
		}
		current = current.FieldByName(field.GoName)
	}
	return current, current.IsValid()
}

func isEmptyValue(value reflect.Value) bool {
	for value.IsValid() && (value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer) {
		if value.IsNil() {
			return true
		}
		value = value.Elem()
	}
	if !value.IsValid() {
		return true
	}
	switch value.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return value.Len() == 0
	default:
		return value.IsZero()
	}
}
