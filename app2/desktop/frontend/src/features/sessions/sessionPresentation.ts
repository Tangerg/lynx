export function compactPath(path: string): string {
  const parts = path.split(/[\\/]/).filter(Boolean);
  return parts.length <= 2 ? path : `…/${parts.slice(-2).join("/")}`;
}

export function workspaceName(path: string): string {
  const parts = path.split(/[\\/]/).filter(Boolean);
  return parts.at(-1) ?? path;
}

export function sessionStatus(
  status: string,
  t: Translate = translateEnglish,
): string {
  if (status === "running") return t("session.status.running");
  if (status === "waiting") return t("session.status.waiting");
  if (status === "idle") return t("session.status.idle");
  return status;
}

export function formatUpdatedAt(
  value: string,
  format: (
    value: Date,
    options?: Intl.DateTimeFormatOptions,
  ) => string = (date, options) =>
    new Intl.DateTimeFormat("en", options).format(date),
): string {
  const date = new Date(value);
  return Number.isNaN(date.valueOf())
    ? ""
    : format(date, {
        month: "short",
        day: "numeric",
      });
}
import {
  translateEnglish,
  type Translate,
} from "../localization/Localization";
