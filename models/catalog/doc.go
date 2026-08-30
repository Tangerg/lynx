// Package catalog exposes the embedded model catalog: model identity, pricing,
// capabilities, modalities, and token limits. It is provider reference data,
// independent from the Core model invocation protocols.
//
// The catalog answers questions about a model — what it costs, which modalities
// it accepts, how many tokens it takes — without invoking anything. A caller
// that needs to call a model uses the matching models/<provider> module.
//
//	entry, found := catalog.Default.Lookup("openai", "gpt-4o")
//	if !found {
//	    return fmt.Errorf("unknown model")
//	}
//
// The data is embedded, so a lookup performs no I/O and cannot fail on a
// network. Updating it is a repository change rather than a runtime fetch.
//
// This module holds no provider SDK and implements no Core Model contract, and
// it never becomes a routing layer: choosing a model is the caller's decision
// and the catalog only supplies the facts.
//
// See README.md for usage and ARCHITECTURE.md for what this module owns.
package catalog
