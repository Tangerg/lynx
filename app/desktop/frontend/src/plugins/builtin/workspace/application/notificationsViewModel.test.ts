import { describe, expect, it } from "vitest";
import type { NotificationEntry } from "@/plugins/sdk";
import {
  notificationDotTone,
  notificationsSubtext,
  notificationsViewModel,
} from "./notificationsViewModel";

const entry = (over: Partial<NotificationEntry>): NotificationEntry => ({
  id: 1,
  plugin: "workspace",
  level: "info",
  message: "message",
  timestamp: 0,
  ...over,
});

describe("notificationsViewModel", () => {
  it("orders newest first and counts unread entries", () => {
    const oldest = entry({ id: 1, message: "oldest", timestamp: 10 });
    const dismissed = entry({ id: 2, message: "dismissed", timestamp: 20, dismissed: true });
    const newest = entry({ id: 3, message: "newest", timestamp: 30 });

    expect(notificationsViewModel([oldest, dismissed, newest])).toEqual({
      entries: [newest, dismissed, oldest],
      unreadCount: 2,
      totalCount: 3,
      isEmpty: false,
    });
  });

  it("projects the empty feed", () => {
    expect(notificationsViewModel([])).toEqual({
      entries: [],
      unreadCount: 0,
      totalCount: 0,
      isEmpty: true,
    });
  });
});

describe("notifications view helpers", () => {
  it("builds header subtext", () => {
    expect(notificationsSubtext({ unreadCount: 1, totalCount: 3 })).toBe("1 unread · 3 total");
  });

  it("maps notification levels to status dot tones", () => {
    expect(notificationDotTone("error")).toBe("err");
    expect(notificationDotTone("warn")).toBe("waiting");
    expect(notificationDotTone("info")).toBe("idle");
  });
});
