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
