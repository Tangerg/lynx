package core

import (
	"fmt"
	"strconv"
	"strings"
)

// Semver is the agent-local semantic version (used for Agent.Version). It's
// kept simple — no operator overloads, no range matching, just a stable
// printable identifier.
type Semver struct {
	Major      int
	Minor      int
	Patch      int
	PreRelease string
	Build      string
}

func (v Semver) String() string {
	out := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.PreRelease != "" {
		out += "-" + v.PreRelease
	}
	if v.Build != "" {
		out += "+" + v.Build
	}
	return out
}

// Less compares two semvers ignoring PreRelease/Build (lexical-only) — adequate
// for "did this version bump?" checks; not for full PEP-440-style ordering.
func (v Semver) Less(o Semver) bool {
	if v.Major != o.Major {
		return v.Major < o.Major
	}
	if v.Minor != o.Minor {
		return v.Minor < o.Minor
	}
	return v.Patch < o.Patch
}

// ParseSemver accepts "M.m.p[-pre][+build]" forms. Anything malformed yields a
// zero-value Semver — DSL callers should validate ahead of time.
func ParseSemver(s string) Semver {
	if s == "" {
		return Semver{}
	}
	v := Semver{}

	// Strip build metadata first.
	if plus := strings.Index(s, "+"); plus >= 0 {
		v.Build = s[plus+1:]
		s = s[:plus]
	}
	// Then prerelease tag.
	if dash := strings.Index(s, "-"); dash >= 0 {
		v.PreRelease = s[dash+1:]
		s = s[:dash]
	}
	parts := strings.Split(s, ".")
	if len(parts) >= 1 {
		v.Major, _ = strconv.Atoi(parts[0])
	}
	if len(parts) >= 2 {
		v.Minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) >= 3 {
		v.Patch, _ = strconv.Atoi(parts[2])
	}
	return v
}
