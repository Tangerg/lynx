// Lightweight i18n — no library, ~60 lines.
//
// Why not react-i18next: that package + its peers add ~120KB to the
// bundle and require a Provider in the tree. For an in-app desktop UI
// with two locales and a few dozen strings, this is overkill. The
// helper below uses `useSyncExternalStore` so any component that calls
// `useT()` re-renders when the user changes locale.
//
// Adding a string: drop it into both locales in `messages` below. The
// `t()` key falls back through  active locale → English → key string,
// so a missing translation never crashes the UI.

import { useSyncExternalStore } from "react";

export type Locale = "en" | "zh";

const STORAGE_KEY = "lyra.locale";

// User-facing strings, grouped by surface. Keep keys hierarchical
// (surface.element.purpose) so future search-and-replace stays scoped.
const messages: Record<Locale, Record<string, string>> = {
  en: {
    "common.cancel": "Cancel",
    "common.confirm": "Confirm",
    "common.close": "Close",
    "common.retry": "Retry",
    "common.search": "Search",

    // Sidebar
    "sidebar.search.placeholder": "Search · files · commands",
    "sidebar.search.label": "Search files and commands",
    "sidebar.section.projects": "Projects",
    "sidebar.section.sessions": "Sessions",
    "sidebar.action.addProject": "Add project",
    "sidebar.action.collapse": "Collapse to rail",
    "sidebar.action.expand": "Expand sidebar",
    "sidebar.action.newSession": "New session",
    "sidebar.action.searchHint": "Search (⌘K)",
    "sidebar.action.tools": "Tools / MCP",
    "sidebar.action.settings": "Settings",
    "sidebar.user.menuLabel": "Account menu",

    // Composer
    "composer.input.label": "Message composer",
    "composer.placeholder.fallback": "Ask, plan, or paste a stack trace…  /  to run a command",
    "composer.send": "Send",
    "composer.mode": "Composer mode",
    "composer.attachFile": "Attach file",
    "composer.switchModel": "Switch model",

    // Chat error boundary
    "chat.error.title": "Render error",
    "chat.error.retry": "Retry",

    // Welcome screen
    "welcome.eyebrow": "agent ready",
    "welcome.title": "What can I help you build today.",
    "welcome.sub": "Start a conversation, paste a stack trace, or pick a suggestion below.",
    "welcome.suggest.refactor": "Plan a refactor",
    "welcome.suggest.refactor.prompt": "Help me plan a refactor of ",
    "welcome.suggest.search": "Search the codebase",
    "welcome.suggest.search.prompt": "Search the codebase for ",
    "welcome.suggest.review": "Review recent changes",
    "welcome.suggest.review.prompt": "Review my recent changes on ",
    "welcome.suggest.checklist": "Draft a checklist",
    "welcome.suggest.checklist.prompt": "Draft a checklist for ",

    // Settings
    "settings.title": "Settings",
    "settings.pane.appearance": "Appearance",
    "settings.pane.plugins": "Plugins",
    "settings.pane.language": "Language",
    "settings.theme": "Theme",
    "settings.theme.sub":
      "Pick a color theme. Plugins can register more — they show up here automatically.",
    "settings.accent": "Accent",
    "settings.accent.sub": "Functional highlight color — play / active / CTA.",
    "settings.language.label": "Language",
    "settings.language.sub": "Interface language. More locales can be added via plugins.",

    // Icon gallery
    "iconGallery.filterLabel": "Filter icons by name",
    "iconGallery.filterPlaceholder": "Filter by name…",
    "iconGallery.empty": 'No icons match "{q}".',
  },
  zh: {
    "common.cancel": "取消",
    "common.confirm": "确认",
    "common.close": "关闭",
    "common.retry": "重试",
    "common.search": "搜索",

    "sidebar.search.placeholder": "搜索 · 文件 · 命令",
    "sidebar.search.label": "搜索文件和命令",
    "sidebar.section.projects": "项目",
    "sidebar.section.sessions": "会话",
    "sidebar.action.addProject": "添加项目",
    "sidebar.action.collapse": "收起到边栏",
    "sidebar.action.expand": "展开边栏",
    "sidebar.action.newSession": "新建会话",
    "sidebar.action.searchHint": "搜索 (⌘K)",
    "sidebar.action.tools": "工具 / MCP",
    "sidebar.action.settings": "设置",
    "sidebar.user.menuLabel": "账号菜单",

    "composer.input.label": "消息输入框",
    "composer.placeholder.fallback": "提问、规划，或粘贴一段错误堆栈…  斜杠 / 运行命令",
    "composer.send": "发送",
    "composer.mode": "输入模式",
    "composer.attachFile": "添加附件",
    "composer.switchModel": "切换模型",

    "chat.error.title": "渲染出错",
    "chat.error.retry": "重试",

    "welcome.eyebrow": "agent 就绪",
    "welcome.title": "今天想构建点什么？",
    "welcome.sub": "开始对话，粘贴一段错误堆栈，或从下方的建议中挑选。",
    "welcome.suggest.refactor": "规划一次重构",
    "welcome.suggest.refactor.prompt": "帮我规划重构: ",
    "welcome.suggest.search": "搜索代码库",
    "welcome.suggest.search.prompt": "在代码库中搜索: ",
    "welcome.suggest.review": "查看最近改动",
    "welcome.suggest.review.prompt": "查看我最近的改动: ",
    "welcome.suggest.checklist": "起草一份清单",
    "welcome.suggest.checklist.prompt": "起草清单: ",

    "settings.title": "设置",
    "settings.pane.appearance": "外观",
    "settings.pane.plugins": "插件",
    "settings.pane.language": "语言",
    "settings.theme": "主题",
    "settings.theme.sub": "选择一个配色方案。插件可以注册更多主题——会自动出现在这里。",
    "settings.accent": "强调色",
    "settings.accent.sub": "功能性高亮——运行、激活、主要操作。",
    "settings.language.label": "语言",
    "settings.language.sub": "界面语言。可以通过插件添加更多语言。",

    "iconGallery.filterLabel": "按名称筛选图标",
    "iconGallery.filterPlaceholder": "按名称筛选…",
    "iconGallery.empty": '没有匹配 "{q}" 的图标。',
  },
};

// Locale defaults: persisted value → browser language → English.
function detectInitial(): Locale {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored === "en" || stored === "zh") return stored;
  } catch {
    /* ignore */
  }
  const nav = typeof navigator !== "undefined" ? navigator.language : "";
  return nav.toLowerCase().startsWith("zh") ? "zh" : "en";
}

let current: Locale = detectInitial();
const listeners = new Set<() => void>();

export function getLocale(): Locale {
  return current;
}

export function setLocale(loc: Locale): void {
  if (loc === current) return;
  current = loc;
  try {
    localStorage.setItem(STORAGE_KEY, loc);
  } catch {
    /* ignore */
  }
  // Set <html lang="…"> so SR and CSS `:lang()` selectors track too.
  if (typeof document !== "undefined") {
    document.documentElement.lang = loc === "zh" ? "zh-CN" : "en";
  }
  listeners.forEach((l) => l());
}

// Initialise <html lang> on first load.
if (typeof document !== "undefined") {
  document.documentElement.lang = current === "zh" ? "zh-CN" : "en";
}

/** Lookup with active locale → English fallback → key passthrough. */
export function t(key: string, params?: Record<string, string | number>): string {
  let msg = messages[current][key] ?? messages.en[key] ?? key;
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      msg = msg.replaceAll(`{${k}}`, String(v));
    }
  }
  return msg;
}

const subscribe = (cb: () => void) => {
  listeners.add(cb);
  return () => listeners.delete(cb);
};
const getSnapshot = () => current;

/** Reactive locale hook — components using `useT()` re-render on change. */
export function useLocale(): Locale {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}

/** Hook that returns a translate function bound to the live locale. */
export function useT(): typeof t {
  useLocale();
  return t;
}

export const LOCALES: ReadonlyArray<{ id: Locale; label: string }> = [
  { id: "en", label: "English" },
  { id: "zh", label: "简体中文" },
];
