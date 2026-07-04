// A hairline outline for media/thumbnails — theme-preview swatches, composer
// image chips, inlined message images. A 0.5px edge that reads white-on-dark /
// black-on-light so the media's own rectangle is separated from the surface
// without a heavy border (the skill's "no cheap lines" rule keeps it to a
// physical-pixel hairline). Compose onto the element's size / radius /
// object-fit at the callsite via cn().
export const MEDIA_OUTLINE = "border-[0.5px] border-white/10 light:border-black/10";
