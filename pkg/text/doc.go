// Package text provides string-line manipulation helpers and a small
// fluent template renderer.
//
// Line operations: [Lines], [AlignToLeft], [AlignToRight], [AlignCenter],
// [TrimAdjacentBlankLines], [DeleteTopLines], [DeleteBottomLines].
//
// Templating: [Renderer] (chainable, caches the last rendered result)
// or the one-shot [Render] / [MustRender] functions.
package text
