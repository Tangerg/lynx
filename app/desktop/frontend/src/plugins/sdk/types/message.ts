// Message + content-block plugin surface.

import type { ComponentType } from "react";
import type {
  ContentBlock,
  ContentBlockKind,
  ContentBlockMap,
} from "@/plugins/sdk/types/contentBlock";

/**
 * One source citation surfaced as a `[n]` marker in assistant prose. The
 * markdown citation pipeline (rehypeCitations + the <sup> renderer) reads a
 * per-message list of these via CitationContext; the badge hover-shows the
 * source. The list itself is contributed per-message by MESSAGE_CITATION_SOURCE
 * extensions, so the kernel owns no knowledge of which block kind produces
 * citations (the search-block plugin maps its results into these).
 */
export interface Citation {
  /** 1-indexed marker (matches the `[n]` in source markdown). */
  index: number;
  domain: string;
  title: string;
  snippet: string;
}

/**
 * Maps a message's content blocks to the citations they imply. Contributions
 * are concatenated (in registration order) into the per-message registry, so
 * `index` continuity is the kernel's job, not each source's.
 */
export type CitationSource = (blocks: ContentBlock[]) => Citation[];

/**
 * Renderer props for a specific content-block kind. The `block` prop is
 * typed to exactly the variant whose `kind` matches K, so plugins get
 * autocomplete on the block payload fields.
 */
export interface ContentBlockRendererProps<K extends ContentBlockKind> {
  block: ContentBlockMap[K];
}
export type ContentBlockRenderer<K extends ContentBlockKind> = ComponentType<
  ContentBlockRendererProps<K>
>;

/**
 * A message role identity — display name + avatar icon. Used by
 * MessageBlock to render the message header consistently. Built-in roles
 * are `user`, `assistant`, `system`; a plugin can register more (e.g. a
 * `developer` role with a wrench icon).
 */
export interface MessageRoleSpec {
  /** Stable id — matches `Message.role`. */
  id: string;
  /** Header label shown next to the timestamp. */
  displayName: string;
  /** Icon name rendered inside the avatar bubble. */
  icon?: string;
  /**
   * Variant on the Avatar primitive — controls the bubble style. Two
   * built-ins exist today: `msg-user` and `msg-agent`. Plugins can stick
   * with one of those or rely on default styling.
   */
  avatarVariant?: "msg-user" | "msg-agent" | string;
}
