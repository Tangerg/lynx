package dispatch

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ProblemChannel names where a first-party ProblemData type may ride. RPC
// problems have a numeric JSON-RPC code; execution problems ride run/item/tool
// outcomes; inline-status problems ride a successful query result.
type ProblemChannel string

const (
	ProblemChannelRPC          ProblemChannel = "rpc"
	ProblemChannelExecution    ProblemChannel = "execution"
	ProblemChannelInlineStatus ProblemChannel = "inlineStatus"
)

// ProblemContract is the single first-party error catalog. Required and Optional
// describe the complete ProblemData frame for Type; Channels describe its legal
// carriers. The union generator and manifest consume the same rows, so adding a
// problem cannot update validation while silently omitting discovery metadata.
type ProblemContract struct {
	Type     string
	Channels []ProblemChannel
	Required []string
	Optional []string
}

var problemContracts = mustProblemContracts()

// ProblemContracts returns a snapshot of the first-party error catalog.
func ProblemContracts() []ProblemContract {
	out := slices.Clone(problemContracts)
	for index := range out {
		out[index].Channels = slices.Clone(out[index].Channels)
		out[index].Required = slices.Clone(out[index].Required)
		out[index].Optional = slices.Clone(out[index].Optional)
	}
	return out
}

// ProblemTypesFor returns the stable symbolic vocabulary of one carrier.
func ProblemTypesFor(channel ProblemChannel) []string {
	switch channel {
	case ProblemChannelRPC, ProblemChannelExecution, ProblemChannelInlineStatus:
	default:
		panic(fmt.Sprintf("dispatch: unknown problem channel %q", channel))
	}
	var out []string
	for _, contract := range problemContracts {
		if slices.Contains(contract.Channels, channel) {
			out = append(out, contract.Type)
		}
	}
	return out
}

func mustProblemContracts() []ProblemContract {
	byType := make(map[string]*ProblemContract)
	add := func(channel ProblemChannel, types ...string) {
		for _, problemType := range types {
			contract := byType[problemType]
			if contract == nil {
				contract = &ProblemContract{Type: problemType}
				byType[problemType] = contract
			}
			if !slices.Contains(contract.Channels, channel) {
				contract.Channels = append(contract.Channels, channel)
			}
		}
	}

	for _, spec := range rpcErrorSpecs {
		add(ProblemChannelRPC, spec.sentinel.Error())
	}
	add(ProblemChannelExecution,
		protocol.ProblemInternalError,
		protocol.ProblemRunLost,
		protocol.ProblemAgentStuck,
		protocol.ProblemRateLimited,
		protocol.ProblemInvalidAPIKey,
		protocol.ProblemTimeout,
		protocol.ProblemProviderUnavailable,
		protocol.ProblemProviderRejected,
		protocol.ProblemDeniedByUser,
		protocol.ProblemToolFailed,
		protocol.ProblemChildRunCanceled,
	)
	add(ProblemChannelInlineStatus,
		protocol.ProblemMCPAuthorizationRequired,
		protocol.ProblemMCPAuthorizationFailed,
		protocol.ProblemMCPDialFailed,
		protocol.ProblemProviderNotConfigured,
		protocol.ProblemProviderTestFailed,
	)

	common := []string{"detail", "docUrl"}
	out := make([]ProblemContract, 0, len(byType))
	for _, contract := range byType {
		contract.Optional = slices.Clone(common)
		switch contract.Type {
		case protocol.ErrInvalidParams.Error():
			contract.Optional = append(contract.Optional, "errors")
		case protocol.ErrCapabilityNotNeg.Error():
			contract.Required = []string{"requiredCapabilities"}
		case protocol.ErrSessionHasActiveRun.Error():
			contract.Required = []string{"activeRun"}
		case protocol.ErrIdempotencyInProgress.Error():
			contract.Required = []string{"retryAfterSeconds"}
		case protocol.ProblemRateLimited, protocol.ProblemTimeout, protocol.ProblemProviderUnavailable:
			contract.Optional = append(contract.Optional, "retryAfterSeconds")
		case protocol.ProblemMCPAuthorizationRequired,
			protocol.ProblemMCPAuthorizationFailed,
			protocol.ProblemMCPDialFailed,
			protocol.ProblemProviderNotConfigured,
			protocol.ProblemProviderTestFailed:
			// Inline status is a localization key, not server-authored UI copy.
			contract.Optional = nil
		}
		out = append(out, *contract)
	}
	slices.SortFunc(out, func(left, right ProblemContract) int {
		return cmp.Compare(left.Type, right.Type)
	})
	for index, contract := range out {
		if err := contract.validate(); err != nil {
			panic(fmt.Sprintf("dispatch: invalid problem contract %d: %v", index, err))
		}
	}
	return out
}

func (contract ProblemContract) validate() error {
	if contract.Type == "" {
		return errors.New("problem type is empty")
	}
	if len(contract.Channels) == 0 {
		return fmt.Errorf("problem type %q has no channel", contract.Type)
	}
	for index, channel := range contract.Channels {
		if slices.Contains(contract.Channels[:index], channel) {
			return fmt.Errorf("problem type %q repeats channel %q", contract.Type, channel)
		}
		switch channel {
		case ProblemChannelRPC, ProblemChannelExecution, ProblemChannelInlineStatus:
		default:
			return fmt.Errorf("problem type %q has unknown channel %q", contract.Type, channel)
		}
	}
	return nil
}
