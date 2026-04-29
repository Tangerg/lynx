// Package dataunit provides a strongly-typed [DataSize] for working
// with byte counts and helpers for converting between byte units
// (B, KB, MB, GB, TB) using IEC powers of 1024.
//
// Constructors that accept larger units ([SizeOfKB], [SizeOfMB], …)
// return an error on int64 overflow.
package dataunit
