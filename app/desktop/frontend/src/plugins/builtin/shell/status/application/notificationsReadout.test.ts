import { describe, expect, it } from "vitest";
import { notificationBadgeText, unreadNotificationCount } from "./notificationsReadout";

describe("unreadNotificationCount", () => {
  it("counts only notifications that have not been dismissed", () => {
    expect(
      unreadNotificationCount([
        { dismissed: false },
        { dismissed: true },
        {},
        { dismissed: false },
      ]),
    ).toBe(3);
  });
});

describe("notificationBadgeText", () => {
  it("hides the badge when there are no unread notifications", () => {
    expect(notificationBadgeText(0)).toBeNull();
    expect(notificationBadgeText(-1)).toBeNull();
  });

  it("caps double-digit unread counts", () => {
    expect(notificationBadgeText(3)).toBe("3");
    expect(notificationBadgeText(10)).toBe("9+");
  });
});
