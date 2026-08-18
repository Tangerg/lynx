import type { LucideIcon } from "lucide-react";
import type { CSSProperties } from "react";
import type { IconSize } from "@/lib/iconScale";
import {
  ArrowDown,
  ArrowLeft,
  ArrowRight,
  ArrowUp,
  Bell,
  Book,
  Bot,
  Bug,
  ChartColumn,
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  ChevronUp,
  CircleHelp,
  Clock,
  Code,
  Command,
  Copy,
  Download,
  Ellipsis,
  Eye,
  File,
  FileDiff,
  FilePlus,
  FileText,
  Folder,
  FolderOpen,
  FolderSearch,
  GitBranch,
  GitFork,
  Globe,
  Image as ImageIcon,
  History,
  List,
  Maximize2,
  MessageSquare,
  Minimize2,
  Moon,
  PanelLeft,
  PanelRight,
  Pause,
  Pencil,
  Play,
  Plus,
  Repeat,
  Search,
  Send,
  Settings,
  Share2,
  Shield,
  SkipBack,
  SkipForward,
  Sparkle,
  Sparkles,
  Square,
  Star,
  Sun,
  Terminal,
  ThumbsDown,
  ThumbsUp,
  Trash2,
  TriangleAlert,
  UnfoldHorizontal,
  User,
  WrapText,
  Wrench,
  X,
  ZoomIn,
  ZoomOut,
  Zap,
  Archive,
  BookOpen,
  Brain,
  CalendarPlus,
  CalendarX,
  ClipboardCheck,
  Crosshair,
  Flag,
  Library,
  ListChecks,
  Map as MapIcon,
  PackageSearch,
  Paperclip,
  Replace,
  ScrollText,
  Target,
  TextSearch,
  Users,
  Webhook,
} from "lucide-react";

// Project-wide icon adapter — app icon name → lucide-react component.
//
// lucide-react gives us:
//   - 1500+ icons available out-of-the-box (Feather-derived, consistent)
//   - tree-shaking: only icons referenced here ship in the bundle
//     (~150-300 bytes per icon)
//   - sane defaults (24x24 viewBox, currentColor stroke, rounded ends)
//   - consistent stroke width without hand-tuning each path
//
// Plugins consume the app icon vocabulary (<Icon name="search" size="sm" />)
// instead of depending on lucide component names directly.

export type IconName =
  | "search"
  | "plus"
  | "chat"
  | "folder"
  | "folder-open"
  | "code"
  | "terminal"
  | "file"
  | "filetext"
  | "send"
  | "send-arrow"
  | "stop"
  | "play"
  | "pause"
  | "settings"
  | "sun"
  | "moon"
  | "share"
  | "more"
  | "x"
  | "check"
  | "branch"
  | "git"
  | "globe"
  | "book"
  | "history"
  | "tool"
  | "sparkle"
  | "thumbs-up"
  | "thumbs-down"
  | "edit"
  | "image"
  | "command"
  | "panel"
  | "panel-l"
  | "user"
  | "spark"
  | "skip-back"
  | "skip-fwd"
  | "minimize"
  | "maximize"
  | "diff"
  | "list"
  | "chart"
  | "clock"
  | "bell"
  | "lightning"
  | "bug"
  | "shield"
  | "loop"
  | "copy"
  | "chevron-up"
  | "chevron-down"
  | "chevron-left"
  | "chevron-right"
  | "panel-r"
  | "arrow-down"
  | "arrow-left"
  | "arrow-right"
  | "arrow-up"
  | "trash"
  | "alert"
  | "eye"
  | "file-plus"
  | "folder-search"
  | "download"
  | "bot"
  | "question"
  | "star"
  // Built-in tools use distinct glyphs so a scrolled transcript preserves the
  // kind of work performed instead of collapsing unrelated calls into one mark.
  | "scroll"
  | "replace"
  | "text-search"
  | "webhook"
  | "library"
  | "book-open"
  | "paperclip"
  | "users"
  | "map"
  | "list-checks"
  | "flag"
  | "brain"
  | "package-search"
  | "archive"
  | "calendar-plus"
  | "calendar-x"
  | "target"
  | "crosshair"
  | "clipboard-check"
  | "unfold-horizontal"
  | "wrap-text"
  | "zoom-in"
  | "zoom-out";

// Mapping from our project's icon vocabulary to lucide components.
// Names on the left are the project's IconName tokens used at every
// callsite; names on the right are the Feather/Lucide-canonical
// equivalents we render under the hood.
const ICON_MAP = {
  search: Search,
  plus: Plus,
  "zoom-in": ZoomIn,
  "zoom-out": ZoomOut,
  chat: MessageSquare,
  folder: Folder,
  "folder-open": FolderOpen,
  code: Code,
  terminal: Terminal,
  file: File,
  filetext: FileText,
  send: Send,
  // The composer sends with an upward arrow, not a paper plane: the plane is the
  // "compose a message" affordance, and using it for both made two different
  // actions wear one glyph.
  "send-arrow": ArrowUp,
  stop: Square,
  play: Play,
  pause: Pause,
  settings: Settings,
  sun: Sun,
  moon: Moon,
  share: Share2,
  more: Ellipsis,
  x: X,
  check: Check,
  branch: GitBranch,
  git: GitFork,
  globe: Globe,
  book: Book,
  history: History,
  tool: Wrench,
  sparkle: Sparkle,
  "thumbs-up": ThumbsUp,
  "thumbs-down": ThumbsDown,
  edit: Pencil,
  image: ImageIcon,
  command: Command,
  // "panel" + "panel-r" are aliases for the right-side panel layout —
  // callsites use either interchangeably.
  panel: PanelRight,
  "panel-l": PanelLeft,
  "panel-r": PanelRight,
  user: User,
  spark: Sparkles,
  "skip-back": SkipBack,
  "skip-fwd": SkipForward,
  minimize: Minimize2,
  maximize: Maximize2,
  diff: FileDiff,
  list: List,
  chart: ChartColumn,
  clock: Clock,
  bell: Bell,
  lightning: Zap,
  bug: Bug,
  shield: Shield,
  loop: Repeat,
  copy: Copy,
  "chevron-up": ChevronUp,
  "chevron-down": ChevronDown,
  "chevron-left": ChevronLeft,
  "chevron-right": ChevronRight,
  "arrow-down": ArrowDown,
  "arrow-left": ArrowLeft,
  "arrow-right": ArrowRight,
  "arrow-up": ArrowUp,
  trash: Trash2,
  alert: TriangleAlert,
  eye: Eye,
  "file-plus": FilePlus,
  "folder-search": FolderSearch,
  download: Download,
  bot: Bot,
  question: CircleHelp,
  star: Star,
  scroll: ScrollText,
  replace: Replace,
  "text-search": TextSearch,
  webhook: Webhook,
  library: Library,
  "book-open": BookOpen,
  paperclip: Paperclip,
  users: Users,
  map: MapIcon,
  "list-checks": ListChecks,
  flag: Flag,
  brain: Brain,
  "package-search": PackageSearch,
  archive: Archive,
  "calendar-plus": CalendarPlus,
  "calendar-x": CalendarX,
  target: Target,
  crosshair: Crosshair,
  "clipboard-check": ClipboardCheck,
  "unfold-horizontal": UnfoldHorizontal,
  "wrap-text": WrapText,
} satisfies Record<IconName, LucideIcon>;

/**
 * The vocabulary as data, for the tables that name a glyph in a plain string.
 *
 * `Icon` itself is typed, so a component naming a glyph the map lacks cannot
 * compile. A registry contribution cannot say that: the tool-icon table is
 * `Record<string, string>` because a plugin contributes into it, so its glyph
 * names are checked by the test that reads this instead of by the compiler.
 */
export const ICON_NAMES: ReadonlySet<IconName> = new Set(Object.keys(ICON_MAP) as IconName[]);

interface Props {
  name: IconName;
  /** A step on the icon ladder (`lib/iconScale.ts`). Defaults to a glyph sized for
   *  body text. Numeric sizes are deliberately not accepted: they are how the app
   *  ended up with eleven of them. */
  size?: IconSize;
  style?: CSSProperties;
  className?: string;
}

/**
 * One glyph at one of the five ladder sizes.
 *
 * Geometry rides CSS custom properties rather than props so a change to the user's
 * base size reaches every glyph without re-rendering anything, and so stroke width
 * stays paired with the size that derives it — the two must move together or the
 * on-screen weight drifts, which is the failure this ladder exists to end.
 */
export function Icon({ name, size = "sm", style, className }: Props) {
  const Glyph = ICON_MAP[name];
  if (!Glyph) return null;
  return (
    <Glyph
      aria-hidden="true"
      className={className}
      style={{
        width: `var(--icon-${size})`,
        height: `var(--icon-${size})`,
        strokeWidth: `var(--icon-stroke-${size})`,
        ...style,
      }}
    />
  );
}
