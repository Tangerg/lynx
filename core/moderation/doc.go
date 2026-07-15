// Package moderation defines the serializable content-moderation protocol and
// its single-method [Model] capability.
//
// A Request can classify multiple texts. Each Result contains category-level
// Verdict values, while Categories.Flagged provides the aggregate decision.
// Provider-only options use Options.Set so Extra remains JSON-safe; Request has
// no arbitrary parameter bag. Implementations and defaults live outside Core.
package moderation
