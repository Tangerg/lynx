import { cn } from "@/lib/classNames";

/**
 * A path, truncated from the LEFT so the filename survives.
 *
 * A path is not a string with a slash in it: the part a reader is looking for is
 * at the end. Plain `truncate` cuts there, which turns
 * `src/plugins/builtin/chat/composer/ui/Composer.tsx` into
 * `src/plugins/builtin/chat/co…` — every character of the answer removed and
 * every character of the shared prefix kept.
 *
 * So the directory gets the elastic middle and clips at its own left edge, while
 * the separator and the filename are pinned. The bidi trick that clips a
 * left-to-right string on the left is `direction: rtl` on the elastic part only;
 * the separator lives in its own element so it can't be reordered along with it.
 */
export function FilePath({ path, className }: { path: string; className?: string }) {
  const cut = path.lastIndexOf("/");
  const directory = cut > 0 ? path.slice(0, cut) : "";
  const filename = cut >= 0 ? path.slice(cut + 1) : path;

  return (
    <span className={cn("flex min-w-0 items-baseline", className)} title={path}>
      {directory !== "" && (
        // The elastic part, and it is elastic by GROWING from nothing rather than
        // by shrinking from its full size. That distinction is the whole
        // behaviour. Shrinking shares a deficit in proportion to each item's size,
        // so the filename always gave up something too — and since an ellipsis
        // fires on any overflow at all, a residual of a fraction of a pixel still
        // cost it a whole character: `…/kernel/panel/DockResizer.tsx` arrived as
        // `…/DockResizer.t…`, with the answer clipped to save six pixels of
        // prefix. At a zero basis there is no deficit to share while the box can
        // still hold the filename, so the filename is untouched until the
        // directory has been driven to nothing — which is what "last resort"
        // below has always claimed. `max-w-max` stops the growth at the
        // directory's own length, or a short path would push its filename to the
        // far edge of whatever box it was handed.
        <span dir="rtl" className="min-w-0 max-w-max flex-1 truncate text-left text-fg-faint">
          {/* The `rtl` above buys the ellipsis on the left, and it costs the
              string's own order unless this isolates it back. `/Users/…` opens
              with a bidi-NEUTRAL: with nothing but the RTL paragraph on its left
              to take direction from, that slash resolves to RTL and is reordered
              to the far right of the run — where it landed against the separator
              and rendered every absolute path in the app as `…/application//name`.
              A `dir` attribute makes this an isolate, so the reordering stops at
              its boundary while the box it sits in still clips where it did. */}
          <span dir="ltr">{directory}</span>
        </span>
      )}
      {/* Rendered for a root-level path too (`/etc` has no directory but the slash
          is part of the name), which is why it is not inside the branch above. */}
      {cut >= 0 && <span className="shrink-0 text-fg-faint">/</span>}
      {/* The filename is pinned against the DIRECTORY, not against the box: in a column
          narrower than the name itself, `shrink-0` made this the row's min-content and
          pushed the whole row past its container. It shrinks as a last resort, with an
          ellipsis, which is still the right end to lose characters from. */}
      <span className="min-w-0 shrink truncate">{filename}</span>
    </span>
  );
}
