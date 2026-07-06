package mcpserver

import "strings"

// injectionEnvKeys are environment variables that hijack the dynamic linker or a
// language runtime of a spawned process. They have no legitimate use in an MCP
// server's stdio env and are a code-injection vector: a workspace-supplied
// config that set one of these would load attacker code into the server
// subprocess. Matched case-insensitively — the canonical form is upper-case, but
// a case variant is still honored by some loaders / on case-insensitive OSes.
//
// Deliberately narrow: only the no-legitimate-use linker/loader keys. PATH,
// NODE_OPTIONS, PYTHONPATH and friends are NOT here — a server config may set
// them for benign reasons, and dropping them would break legitimate servers.
var injectionEnvKeys = map[string]struct{}{
	"LD_PRELOAD":            {},
	"LD_LIBRARY_PATH":       {},
	"LD_AUDIT":              {},
	"DYLD_INSERT_LIBRARIES": {},
	"DYLD_LIBRARY_PATH":     {},
	"DYLD_FRAMEWORK_PATH":   {},
}

// SafeEnv returns [Server.Env] with the dynamic-linker / runtime injection keys
// (see [injectionEnvKeys]) removed, so a spawned stdio server can't be hijacked
// through a poisoned env entry. The dial layer flattens the result before
// spawning. It returns a fresh map — never mutates Env — and preserves the
// empty case (nil Env → nil result).
func (s Server) SafeEnv() map[string]string {
	if len(s.Env) == 0 {
		return s.Env
	}
	out := make(map[string]string, len(s.Env))
	for k, v := range s.Env {
		if _, blocked := injectionEnvKeys[strings.ToUpper(k)]; blocked {
			continue
		}
		out[k] = v
	}
	return out
}
