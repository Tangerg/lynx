// Package queryparam holds tiny helpers for building HTTP query
// parameter maps in the shape resty expects (map[string]string).
// Shared by GET-style search providers — see websearch/jina and
// websearch/brave for callers.
package queryparam

import (
	"strconv"
	"strings"
)

// AddStr writes (k, v) when v is non-empty.
func AddStr(p map[string]string, k, v string) {
	if v != "" {
		p[k] = v
	}
}

// AddInt writes (k, itoa(v)) when v > 0.
func AddInt(p map[string]string, k string, v int) {
	if v > 0 {
		p[k] = strconv.Itoa(v)
	}
}

// AddBool writes (k, "true") only when v is true; absence implies false.
func AddBool(p map[string]string, k string, v bool) {
	if v {
		p[k] = "true"
	}
}

// AddCSV writes (k, joined) when v is non-empty.
func AddCSV(p map[string]string, k string, v []string) {
	if len(v) > 0 {
		p[k] = strings.Join(v, ",")
	}
}
