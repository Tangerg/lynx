// Package stream provides typed, context-aware streaming primitives.
//
// [Stream] is a closeable bidirectional pipe of T values backed by a
// Go channel. Use [NewStream] to create one, optionally specifying a
// buffer size; reads observe [io.EOF] after Close drains.
//
// Beyond the basic stream, the package offers composable readers and
// writers: [OfSliceReader], [OfChannelReader], [Pipe], [TeeReader],
// [MultiReader], [MultiWriter], plus functional combinators
// [Map], [Filter], [FlatMap], and [Distinct].
package stream
