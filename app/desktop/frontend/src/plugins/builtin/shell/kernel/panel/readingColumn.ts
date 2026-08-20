// The reading column, in two halves, because the split is what keeps the
// transcript and the composer the same width.
//
// COLUMN is the box: the measure plus the gutter beside it. GUTTER is that
// gutter, and it is applied by whatever draws inside the box — each message, the
// composer, a banner — never by the scroller's content wrapper.
//
// Putting the gutter on the wrapper instead is the obvious thing and it is
// wrong twice. The composer wears the same box without it, so the text a user
// reads comes out 40px narrower than the input they type into. And it makes each
// message's box hug its own text exactly, which matters because a message
// carries `content-visibility` — paint containment clips at that box's edge, so
// the action bar's negative optical inset was being sliced off. With the gutter
// inside the message, the containment box is the full column and the bar has
// somewhere to hang.
export const READING_COLUMN = "mx-auto w-full max-w-[var(--reading-column-max)]";

export const READING_GUTTER =
  "px-[var(--density-column-gutter)] sm:px-[var(--density-column-gutter-wide)]";

// How much of the transcript the floating composer covers, and the clearance
// the transcript's tail takes from it. Two halves of one contract, kept in one
// file because they cannot be expressed as one thing: Tailwind reads source
// text, so the class has to spell the property that the constant names. Filed
// apart, they drift, and the symptom is a last message nobody can scroll out
// from under. The extra pixel absorbs the browser's integer scrollTop rounding
// when the observed overlay and the transcript paint on fractional CSS pixels;
// the visible gap therefore never rounds below the intended rem.
export const COMPOSER_OVERLAY_PROPERTY = "--composer-overlay";

export const COMPOSER_CLEARANCE = "pb-[calc(var(--composer-overlay,0px)+1rem+1px)]";
