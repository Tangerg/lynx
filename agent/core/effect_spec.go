package core

import (
	"maps"
	"slices"
)

// EffectSpec maps condition keys to required (or produced) Determinations.
// It represents both Action.Preconditions ("what must hold before I can run")
// and Action.Effects ("what becomes true after I run"). The same shape is
// reused for Goal.Preconditions.
//
// A nil EffectSpec is a valid, read-only empty value; all helpers accept it
// without panicking.
type EffectSpec map[string]Determination

// Clone returns a deep copy. A nil receiver yields nil so callers can chain
// "spec.Clone()" without a guard.
func (s EffectSpec) Clone() EffectSpec {
	if s == nil {
		return nil
	}
	return maps.Clone(s)
}

// Merge layers other on top of s — keys in other win. Returns a new map;
// neither input is modified.
func (s EffectSpec) Merge(other EffectSpec) EffectSpec {
	out := make(EffectSpec, len(s)+len(other))
	maps.Copy(out, s)
	maps.Copy(out, other)
	return out
}

// Keys returns sorted condition keys for stable iteration (used by HashKey
// computations and debug printing).
func (s EffectSpec) Keys() []string {
	out := slices.Collect(maps.Keys(s))
	slices.Sort(out)
	return out
}

// Set returns a copy with key=value applied — intended for fluent
// construction, not for mutating an existing spec in place.
func (s EffectSpec) Set(key string, value Determination) EffectSpec {
	out := s.Clone()
	if out == nil {
		out = EffectSpec{}
	}
	out[key] = value
	return out
}
