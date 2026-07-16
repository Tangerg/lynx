package workflow

import (
	"cmp"
	"context"
	"errors"
	"slices"

	"github.com/Tangerg/lynx/agent/core"
)

// ConsensusConfig is a [ScatterGatherConfig] specialization where every
// generator returns the same Element type and a tally function picks
// the consensus winner. Use for "ask 3 LLMs, take the answer ≥ 2
// agree on" or "ensemble vote among models".
//
// In is the workflow input, fed to every voter. Element is the type
// each voter returns (often a string label or small struct that can
// be compared via Key).
//
// ConsensusConfig configures a consensus workflow — voters are plain
// functions; users inject different chatclient.Client values via closure.
type ConsensusConfig[In, Element any] struct {
	// Name names the produced agent + its goal. Required.
	Name string

	// Description is the agent's human-facing summary.
	Description string

	// MaxConcurrency caps in-flight voters. <=0 means unbounded.
	MaxConcurrency int

	// Voters is the parallel ensemble. Each receives In and
	// returns an Element. Must be non-empty.
	Voters []func(ctx context.Context, process *core.ProcessContext, input In) (Element, error)

	// Key projects each Element to a comparable string used to
	// tally votes. Required when Element isn't directly comparable
	// (string / numeric); for comparable types, supply
	// [DefaultKey] or a custom projector that picks the salient
	// field.
	Key func(Element) string
}

// Consensus compiles config into an agent that runs every voter,
// tallies via Key, and returns the Element whose Key occurs most
// often. Ties are broken by voter order (the earliest voter whose
// Key tied for the lead wins).
//
// Returns an error on missing Name / empty Voters / nil Key.
func Consensus[In, Element any](config ConsensusConfig[In, Element]) (*core.Agent, error) {
	if config.Name == "" {
		return nil, errors.New("workflow.Consensus: Name must not be empty")
	}
	if len(config.Voters) == 0 {
		return nil, errors.New("workflow.Consensus: Voters must not be empty")
	}
	if config.Key == nil {
		return nil, errors.New("workflow.Consensus: Key must not be nil")
	}

	return ScatterGather(ScatterGatherConfig[In, Element, Element]{
		Name:           config.Name,
		Description:    config.Description,
		MaxConcurrency: config.MaxConcurrency,
		Generators:     config.Voters,
		Joiner: func(_ context.Context, _ *core.ProcessContext, votes []Element) (Element, error) {
			return pickConsensus(votes, config.Key), nil
		},
	})
}

// DefaultKey is a Key projector for Element types whose own string
// representation is the right tally key (typically string Elements).
// Use for `ConsensusConfig[..., string]{Key: workflow.DefaultKey[string]}`.
func DefaultKey[Element ~string](element Element) string { return string(element) }

// pickConsensus tallies votes by key and returns the Element whose
// key has the highest count. Stable: on a tie, the first-seen
// Element wins (preserves voter order).
func pickConsensus[Element any](votes []Element, key func(Element) string) Element {
	if len(votes) == 0 {
		var zero Element
		return zero
	}

	type bucket struct {
		key        string
		firstIndex int
		count      int
		sample     Element
	}
	buckets := make(map[string]*bucket)
	for index, vote := range votes {
		voteKey := key(vote)
		if existing, ok := buckets[voteKey]; ok {
			existing.count++
			continue
		}
		buckets[voteKey] = &bucket{key: voteKey, firstIndex: index, count: 1, sample: vote}
	}

	ranked := make([]*bucket, 0, len(buckets))
	for _, bucket := range buckets {
		ranked = append(ranked, bucket)
	}
	slices.SortStableFunc(ranked, func(left, right *bucket) int {
		if left.count != right.count {
			return cmp.Compare(right.count, left.count) // higher count first
		}
		return cmp.Compare(left.firstIndex, right.firstIndex) // ties broken by first appearance
	})
	return ranked[0].sample
}
