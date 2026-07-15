// Package media defines the serializable [Media] container shared by every
// modality that accepts non-text payloads.
//
// Use NewBytes for owned inline content, NewURI for an absolute external URI,
// or NewReference for a provider-native identifier. MediaSource is a tagged
// union: exactly one source must match its Kind. Constructors and JSON decoding
// enforce that invariant and validate the MIME type.
package media
