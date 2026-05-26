// Thin wrapper over i18next + react-i18next so the rest of the app
// stays on a stable `useT() / setLocale() / useLocale()` API. Dictionary
// lives below as a single flat-keyed object (no nested namespaces) — the
// helper sets `keySeparator: false` to keep "sidebar.search.label" as a
// literal key instead of treating it as a path.

import i18next from "i18next";
import { initReactI18next, useTranslation } from "react-i18next";

export type Locale = "en" | "zh";

const STORAGE_KEY = "lyra.locale";

const messages: Record<Locale, Record<string, string>> = {
  en: {
    "common.cancel": "Cancel",
    "common.confirm": "Confirm",
    "common.close": "Close",
    "common.retry": "Retry",
    "common.search": "Search",

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

    "composer.input.label": "Message composer",
    "composer.placeholder.fallback": "Ask, plan, or paste a stack trace…  /  to run a command",
    "composer.send": "Send",
    "composer.mode": "Composer mode",
    "composer.attachFile": "Attach file",
    "composer.switchModel": "Switch model",

    "chat.error.title": "Render error",
    "chat.error.retry": "Retry",

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

    "settings.title": "Settings",
    "settings.pane.appearance": "Appearance",
    "settings.pane.plugins": "Plugins",
    "settings.pane.language": "Language",
    "settings.theme": "Theme",
    "settings.theme.sub":
      "Pick a color theme. Plugins can register more — they show up here automatically.",
    "settings.accent": "Accent",
    "settings.accent.sub": "Functional highlight color — play / active / CTA.",
    "settings.accent.custom": "Pick a custom color",
    "settings.font": "Font",
    "settings.font.sub": "Typeface + size. Empty = bundled Geist.",
    "settings.font.ui": "UI",
    "settings.font.code": "Code",
    "settings.font.size": "Size",
    "settings.font.default": "Default",
    "settings.messageStyle": "Message style",
    "settings.messageStyle.sub": "How your own messages render.",
    "settings.messageStyle.bubble": "Bubble",
    "settings.messageStyle.plain": "Plain",
    "settings.language.label": "Language",
    "settings.language.sub": "Interface language. More locales can be added via plugins.",

    "settings.pane.connection": "Connection",
    "settings.connection.title": "Backend",
    "settings.connection.sub": "Where the AG-UI runtime is reachable. Changes apply on the next request.",
    "settings.connection.url": "URL",
    "settings.connection.reset": "Reset to default",

    "iconGallery.filterLabel": "Filter icons by name",
    "iconGallery.filterPlaceholder": "Filter by name…",

    "approval.settled.approved": "Approved · executing",
    "approval.settled.declined": "Declined",
    "approval.required": "Approval required",
    "approval.action.approve": "Approve",
    "approval.action.decline": "Decline",
    "approval.risk.low": "Low risk",
    "approval.risk.medium": "Medium risk",
    "approval.risk.high": "High risk",
    "approval.reversible": "Reversible",
    "approval.permanent": "Permanent",

    "runError.title": "Agent error",
    "runError.action.retry": "Retry",
    "runError.action.timeline": "Open timeline",
    "runError.action.diagnostics": "Diagnostics",
    "runError.action.dismiss": "Dismiss",

    "session.status.running": "Running",
    "session.status.waiting": "Needs input",

    "time.now": "just now",
    "time.minutes_one": "{{count}} minute ago",
    "time.minutes_other": "{{count}} minutes ago",
    "time.hours_one": "{{count}} hour ago",
    "time.hours_other": "{{count}} hours ago",
    "time.days_one": "{{count}} day ago",
    "time.days_other": "{{count}} days ago",

    "iconGallery.empty": 'No icons match "{{q}}".',
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
    "settings.accent.custom": "自定义颜色",
    "settings.font": "字体",
    "settings.font.sub": "字体 + 字号。留空使用内置 Geist。",
    "settings.font.ui": "界面",
    "settings.font.code": "代码",
    "settings.font.size": "字号",
    "settings.font.default": "默认",
    "settings.messageStyle": "消息样式",
    "settings.messageStyle.sub": "你自己消息的渲染方式。",
    "settings.messageStyle.bubble": "气泡",
    "settings.messageStyle.plain": "平铺",
    "settings.language.label": "语言",
    "settings.language.sub": "界面语言。可以通过插件添加更多语言。",

    "settings.pane.connection": "连接",
    "settings.connection.title": "后端",
    "settings.connection.sub": "AG-UI 运行时的地址。下一次请求生效。",
    "settings.connection.url": "URL",
    "settings.connection.reset": "恢复默认",

    "iconGallery.filterLabel": "按名称筛选图标",
    "iconGallery.filterPlaceholder": "按名称筛选…",
    "iconGallery.empty": '没有匹配 "{{q}}" 的图标。',

    "approval.settled.approved": "已批准 · 正在执行",
    "approval.settled.declined": "已拒绝",
    "approval.required": "需要审批",
    "approval.action.approve": "批准",
    "approval.action.decline": "拒绝",
    "approval.risk.low": "低风险",
    "approval.risk.medium": "中风险",
    "approval.risk.high": "高风险",
    "approval.reversible": "可撤销",
    "approval.permanent": "不可撤销",

    "runError.title": "Agent 报错",
    "runError.action.retry": "重试",
    "runError.action.timeline": "查看时间线",
    "runError.action.diagnostics": "诊断",
    "runError.action.dismiss": "忽略",

    "session.status.running": "运行中",
    "session.status.waiting": "等待输入",

    "time.now": "刚才",
    "time.minutes_one": "{{count}} 分钟前",
    "time.minutes_other": "{{count}} 分钟前",
    "time.hours_one": "{{count}} 小时前",
    "time.hours_other": "{{count}} 小时前",
    "time.days_one": "{{count}} 天前",
    "time.days_other": "{{count}} 天前",
  },
};

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

const initial = detectInitial();

void i18next.use(initReactI18next).init({
  resources: {
    en: { translation: messages.en },
    zh: { translation: messages.zh },
  },
  lng: initial,
  fallbackLng: "en",
  // Keys are dotted strings ("sidebar.search.label") — treat them as
  // literal, not as nested paths.
  keySeparator: false,
  nsSeparator: false,
  interpolation: { escapeValue: false },
  returnNull: false,
});

if (typeof document !== "undefined") {
  document.documentElement.lang = initial === "zh" ? "zh-CN" : "en";
}

function getLocale(): Locale {
  return (i18next.resolvedLanguage as Locale | undefined) ?? "en";
}

export function setLocale(loc: Locale): void {
  if (loc === getLocale()) return;
  void i18next.changeLanguage(loc);
  try {
    localStorage.setItem(STORAGE_KEY, loc);
  } catch {
    /* ignore */
  }
  if (typeof document !== "undefined") {
    document.documentElement.lang = loc === "zh" ? "zh-CN" : "en";
  }
}

export function t(key: string, params?: Record<string, string | number>): string {
  return i18next.t(key, params) as string;
}

/** Reactive locale hook — components using this re-render on change. */
export function useLocale(): Locale {
  const { i18n } = useTranslation();
  return (i18n.resolvedLanguage as Locale | undefined) ?? "en";
}

/** Hook returning a translate fn bound to the live locale. The returned
 *  reference is stable across renders (until the language changes) so it's
 *  safe to use in `useMemo` / `useCallback` deps. */
export function useT(): typeof t {
  // Subscribe for re-renders on language change; the module-level `t`
  // reads i18next live so it always sees the new locale.
  useTranslation();
  return t;
}

export const LOCALES: ReadonlyArray<{ id: Locale; label: string }> = [
  { id: "en", label: "English" },
  { id: "zh", label: "简体中文" },
];

/**
 * Merge `dict` into the dictionary for `locale`. Existing keys are
 * overwritten; new keys land alongside the kernel's strings. Used by
 * `host.i18n.addBundle` so plugins can contribute their own labels.
 *
 * i18next has no public per-key removal, so plugin unload doesn't roll
 * the bundle back. In practice this is fine — keys are unreferenced
 * after the plugin's UI is gone, and a same-name reload overwrites
 * cleanly.
 */
export function addLocaleBundle(locale: string, dict: Record<string, string>): void {
  i18next.addResourceBundle(locale, "translation", dict, true, true);
}
