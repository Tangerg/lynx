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
        <span dir="rtl" className="min-w-0 shrink truncate text-left text-fg-faint">
          {directory}
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
