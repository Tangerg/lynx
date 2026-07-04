// A hairline outline for media/thumbnails — theme-preview swatches, composer
// image chips, inlined message images. An inset ink-alpha edge (auto-adapting:
// black-on-light, white-on-dark via the fg token) so the media's own rectangle
// is separated from the surface without a heavy border. `outline` (not
// `border`) adds no layout size and follows the element's box. Compose onto the
// element's size / radius / object-fit at the callsite via cn().
export const MEDIA_OUTLINE = "outline outline-1 -outline-offset-1 outline-fg/10";
