// Package moderation defines the serializable content-moderation protocol and
// its single-method [Model] capability.
//
// A Request can classify multiple texts. Each Result contains category-level
// Verdict values, while Categories.Flagged provides the aggregate decision.
// Provider implementations and policy-specific defaults live outside Core.
package moderation
