// Message + content-block plugin surface.

import type { ComponentType } from "react";
import type { ContentBlockKind, ContentBlockMap } from "@/protocol/agui/viewState";

/**
 * Renderer props for a specific content-block kind. The `block` prop is
 * typed to exactly the variant whose `kind` matches K, so plugins get
 * autocomplete on the block payload fields.
 */
export type ContentBlockRendererProps<K extends ContentBlockKind> = {
  block: ContentBlockMap[K];
};
export type ContentBlockRenderer<K extends ContentBlockKind> = ComponentType<
  ContentBlockRendererProps<K>
>;

/**
 * A message role identity — display name + avatar icon. Used by
 * MessageBlock to render the message header consistently. Built-in roles
 * are `user`, `assistant`, `system`; a plugin can register more (e.g. a
 * `developer` role with a wrench icon).
 */
export type MessageRoleSpec = {
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
};
