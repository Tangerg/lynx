import type { CSSProperties, SVGProps } from "react";

export type IconName =
  | "search" | "plus" | "chat" | "folder" | "code" | "terminal" | "file"
  | "filetext" | "send" | "send-arrow" | "stop" | "play" | "pause"
  | "settings" | "sun" | "moon" | "share" | "more" | "x" | "check"
  | "branch" | "git" | "globe" | "book" | "history" | "tool" | "sparkle"
  | "edit" | "paperclip" | "image" | "command" | "panel" | "panel-l"
  | "user" | "spark" | "skip-back" | "skip-fwd" | "minimize" | "diff"
  | "list" | "lightning" | "bug" | "shield" | "loop" | "copy" | "panel-r"
  | "arrow-down";

type Props = {
  name: IconName;
  size?: number;
  strokeWidth?: number;
  style?: CSSProperties;
  className?: string;
};

export function Icon({ name, size = 16, strokeWidth = 2, style, className }: Props) {
  const p: SVGProps<SVGSVGElement> = {
    width: size,
    height: size,
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    strokeWidth,
    strokeLinecap: "round",
    strokeLinejoin: "round",
    style,
    className,
  };
  switch (name) {
    case "search": return <svg {...p}><circle cx="11" cy="11" r="7" /><path d="m21 21-4.3-4.3" /></svg>;
    case "plus": return <svg {...p}><path d="M12 5v14M5 12h14" /></svg>;
    case "chat": return <svg {...p}><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" /></svg>;
    case "folder": return <svg {...p}><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.7-.9L9.6 3.9A2 2 0 0 0 7.9 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" /></svg>;
    case "code": return <svg {...p}><path d="m16 18 6-6-6-6M8 6l-6 6 6 6" /></svg>;
    case "terminal": return <svg {...p}><path d="m4 17 6-6-6-6M12 19h8" /></svg>;
    case "file": return <svg {...p}><path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7z" /><path d="M14 2v4a2 2 0 0 0 2 2h4" /></svg>;
    case "filetext": return <svg {...p}><path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7z" /><path d="M14 2v4a2 2 0 0 0 2 2h4" /><path d="M10 9H8M16 13H8M16 17H8" /></svg>;
    case "send": return <svg {...p} fill="currentColor" stroke="none"><path d="M3 11.5 21 3l-8.5 18-2-8z" /></svg>;
    case "send-arrow": return <svg {...p}><path d="M12 19V5M5 12l7-7 7 7" /></svg>;
    case "stop": return <svg {...p} fill="currentColor" stroke="none"><rect x="6" y="6" width="12" height="12" rx="2" /></svg>;
    case "play": return <svg {...p} fill="currentColor" stroke="none"><path d="M8 5v14l11-7z" /></svg>;
    case "pause": return <svg {...p} fill="currentColor" stroke="none"><rect x="6" y="4" width="4" height="16" /><rect x="14" y="4" width="4" height="16" /></svg>;
    case "settings": return <svg {...p}><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09a1.65 1.65 0 0 0-1-1.51 1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09a1.65 1.65 0 0 0 1.51-1 1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33h0a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82v0a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" /></svg>;
    case "sun": return <svg {...p}><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" /></svg>;
    case "moon": return <svg {...p}><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" /></svg>;
    case "share": return <svg {...p}><circle cx="18" cy="5" r="3" /><circle cx="6" cy="12" r="3" /><circle cx="18" cy="19" r="3" /><path d="m8.59 13.51 6.83 3.98M15.41 6.51 8.59 10.49" /></svg>;
    case "more": return <svg {...p}><circle cx="12" cy="12" r="1" /><circle cx="19" cy="12" r="1" /><circle cx="5" cy="12" r="1" /></svg>;
    case "x": return <svg {...p}><path d="M18 6 6 18M6 6l12 12" /></svg>;
    case "check": return <svg {...p}><path d="M20 6 9 17l-5-5" /></svg>;
    case "branch": return <svg {...p}><line x1="6" y1="3" x2="6" y2="15" /><circle cx="18" cy="6" r="3" /><circle cx="6" cy="18" r="3" /><path d="M18 9a9 9 0 0 1-9 9" /></svg>;
    case "git": return <svg {...p}><circle cx="12" cy="12" r="10" /><path d="M14.5 9.5 9.5 14.5M9.5 9.5 14.5 14.5" /></svg>;
    case "globe": return <svg {...p}><circle cx="12" cy="12" r="10" /><path d="M2 12h20M12 2a15 15 0 0 1 4 10 15 15 0 0 1-4 10 15 15 0 0 1-4-10 15 15 0 0 1 4-10z" /></svg>;
    case "book": return <svg {...p}><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20" /><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2" /></svg>;
    case "history": return <svg {...p}><path d="M3 12a9 9 0 1 0 3-6.7L3 8" /><path d="M3 3v5h5M12 7v5l4 2" /></svg>;
    case "tool": return <svg {...p}><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94L9 17.25A2.83 2.83 0 1 1 5 13.4l6.78-6.78a6 6 0 0 1 7.94-7.94l-3.76 3.76z" /></svg>;
    case "sparkle": return <svg {...p}><path d="m12 3-1.9 5.8L4 10l6.1 1.5L12 17l1.9-5.5L20 10l-6.1-1.2z" /></svg>;
    case "edit": return <svg {...p}><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" /><path d="M18.5 2.5a2.12 2.12 0 0 1 3 3L12 15l-4 1 1-4z" /></svg>;
    case "paperclip": return <svg {...p}><path d="m21.44 11.05-9.19 9.19a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48" /></svg>;
    case "image": return <svg {...p}><rect x="3" y="3" width="18" height="18" rx="2" /><circle cx="9" cy="9" r="2" /><path d="m21 15-5-5L5 21" /></svg>;
    case "command": return <svg {...p}><path d="M18 3a3 3 0 0 0-3 3v12a3 3 0 0 0 3 3 3 3 0 0 0 3-3 3 3 0 0 0-3-3H6a3 3 0 0 0-3 3 3 3 0 0 0 3 3 3 3 0 0 0 3-3V6a3 3 0 0 0-3-3 3 3 0 0 0-3 3 3 3 0 0 0 3 3h12a3 3 0 0 0 3-3 3 3 0 0 0-3-3z" /></svg>;
    case "panel": return <svg {...p}><rect x="3" y="3" width="18" height="18" rx="2" /><path d="M15 3v18" /></svg>;
    case "panel-l": return <svg {...p}><rect x="3" y="3" width="18" height="18" rx="2" /><path d="M9 3v18" /></svg>;
    case "user": return <svg {...p}><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2" /><circle cx="12" cy="7" r="4" /></svg>;
    case "spark": return <svg {...p} fill="currentColor" stroke="none"><path d="m12 2 2.5 7L22 12l-7.5 3L12 22l-2.5-7L2 12l7.5-3z" /></svg>;
    case "skip-back": return <svg {...p} fill="currentColor" stroke="none"><path d="M19 20 9 12l10-8zM5 19V5" /></svg>;
    case "skip-fwd": return <svg {...p} fill="currentColor" stroke="none"><path d="m5 4 10 8-10 8zM19 5v14" /></svg>;
    case "minimize": return <svg {...p}><path d="M4 14h6v6M20 10h-6V4M14 10l7-7M3 21l7-7" /></svg>;
    case "diff": return <svg {...p}><path d="M12 3v18M3 8h6M3 16h6M15 12l3-3 3 3M18 9v6" /></svg>;
    case "list": return <svg {...p}><path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01" /></svg>;
    case "lightning": return <svg {...p} fill="currentColor" stroke="none"><path d="M13 2 3 14h8l-1 8 10-12h-8z" /></svg>;
    case "bug": return <svg {...p}><path d="m8 2 1.88 1.88M14.12 3.88 16 2M9 7.13v-1a3.003 3.003 0 1 1 6 0v1M12 20c-3.3 0-6-2.7-6-6v-3a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v3c0 3.3-2.7 6-6 6M12 20v-9M6.53 9C4.6 8.8 3 7.1 3 5M6 13H2M3 21c0-2.1 1.7-3.9 3.8-4M20.97 5c0 2.1-1.6 3.8-3.5 4M16 13h4M21 21c0-2.1-1.7-3.9-3.8-4" /></svg>;
    case "shield": return <svg {...p}><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10" /></svg>;
    case "loop": return <svg {...p}><path d="m17 2 4 4-4 4M3 11v-1a4 4 0 0 1 4-4h14M7 22l-4-4 4-4M21 13v1a4 4 0 0 1-4 4H3" /></svg>;
    case "copy": return <svg {...p}><rect x="9" y="9" width="13" height="13" rx="2" ry="2" /><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" /></svg>;
    case "panel-r": return <svg {...p}><rect x="3" y="3" width="18" height="18" rx="2" /><path d="M15 3v18" /></svg>;
    case "arrow-down": return <svg {...p}><path d="M12 5v14M19 12l-7 7-7-7" /></svg>;
    default:
      return null;
  }
}
