// Message plugin surface.

/**
 * A message role identity — display name + avatar icon. Used by
 * MessageBlock to render the message header consistently. Built-in roles
 * are `user`, `assistant`, `system`; a plugin can register more (e.g. a
 * `developer` role with a wrench icon).
 */
export interface MessageRoleSpec {
  /** Stable id — matches `Message.role`. */
  id: string;
  /** Header label shown next to the timestamp — a catalog key, resolved where it
   *  renders (see `CommandSpec.label` for why it isn't resolved text). */
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
