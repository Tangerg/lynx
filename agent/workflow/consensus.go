package workflow

import (
	"context"
	"sort"

	"github.com/Tangerg/lynx/agent/core"
)

// ConsensusSpec is a [ScatterGatherSpec] specialization where every
// generator returns the same Element type and a tally function picks
// the consensus winner. Use for "ask 3 LLMs, take the answer ≥ 2
// agree on" or "ensemble vote among models".
//
// In is the workflow input, fed to every voter. Element is the type
// each voter returns (often a string label or small struct that can
// be compared via Key).
//
// Mirrors embabel's `multimodel/ConsensusBuilder.kt` without the
// Spring multi-model wiring — voters are plain functions; users
// inject different chat.Clients via closure.
type ConsensusSpec[In, Element any] struct {
	// Name names the produced agent + its goal. Required.
	Name string

	// Description is the agent's human-facing summary.
	Description string

	// MaxConcurrency caps in-flight voters. <=0 means unbounded.
	MaxConcurrency int

	// Voters is the parallel ensemble. Each receives In and
	// returns an Element. Must be non-empty.
	Voters []func(ctx context.Context, pc *core.ProcessContext, in In) (Element, error)

	// Key projects each Element to a comparable string used to
	// tally votes. Required when Element isn't directly comparable
	// (string / numeric); for comparable types, supply
	// [DefaultKey] or a custom projector that picks the salient
	// field.
	Key func(Element) string
}

// ConsensusAgent compiles spec into an agent that runs every voter,
// tallies via Key, and returns the Element whose Key occurs most
// often. Ties are broken by voter order (the earliest voter whose
// Key tied for the lead wins).
//
// Panics on missing Name / empty Voters / nil Key.
func ConsensusAgent[In, Element any](spec ConsensusSpec[In, Element]) *core.Agent {
	if spec.Name == "" {
		panic("workflow.ConsensusAgent: Name must not be empty")
	}
	if len(spec.Voters) == 0 {
		panic("workflow.ConsensusAgent: Voters must not be empty")
	}
	if spec.Key == nil {
		panic("workflow.ConsensusAgent: Key must not be nil")
	}

	return ScatterGatherAgent(ScatterGatherSpec[In, Element, Element]{
		Name:           spec.Name,
		Description:    spec.Description,
		MaxConcurrency: spec.MaxConcurrency,
		Generators:     spec.Voters,
		Joiner: func(_ context.Context, _ *core.ProcessContext, votes []Element) (Element, error) {
			return pickConsensus(votes, spec.Key), nil
		},
	})
}

// DefaultKey is a Key projector for Element types whose own string
// representation is the right tally key (typically string Elements).
// Use for `ConsensusSpec[..., string]{Key: workflow.DefaultKey[string]}`.
func DefaultKey[Element ~string](e Element) string { return string(e) }

// pickConsensus tallies votes by key and returns the Element whose
// key has the highest count. Stable: on a tie, the first-seen
// Element wins (preserves voter order).
func pickConsensus[Element any](votes []Element, key func(Element) string) Element {
	if len(votes) == 0 {
		var zero Element
		return zero
	}

	type bucket struct {
		key      string
		firstIdx int
		count    int
		sample   Element
	}
	buckets := make(map[string]*bucket)
	for i, v := range votes {
		k := key(v)
		if b, ok := buckets[k]; ok {
			b.count++
			continue
		}
		buckets[k] = &bucket{key: k, firstIdx: i, count: 1, sample: v}
	}

	all := make([]*bucket, 0, len(buckets))
	for _, b := range buckets {
		all = append(all, b)
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].count != all[j].count {
			return all[i].count > all[j].count
		}
		return all[i].firstIdx < all[j].firstIdx
	})
	return all[0].sample
}
