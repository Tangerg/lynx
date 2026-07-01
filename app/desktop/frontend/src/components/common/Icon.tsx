import type { LucideIcon } from "lucide-react";
import type { CSSProperties } from "react";
import {
  AlertTriangle,
  ArrowDown,
  Book,
  Bot,
  Bug,
  ChartColumn,
  Check,
  ChevronDown,
  ChevronRight,
  ChevronUp,
  Code,
  Command,
  Copy,
  Download,
  Eye,
  File,
  FileDiff,
  FilePlus,
  FileText,
  Folder,
  FolderSearch,
  GitBranch,
  GitFork,
  Globe,
  History,
  Image as ImageIcon,
  List,
  Maximize2,
  MessageCircleQuestion,
  MessageSquare,
  Minimize2,
  Moon,
  MoreHorizontal,
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
  Star,
  ThumbsDown,
  ThumbsUp,
  Square,
  Sun,
  Terminal,
  Trash2,
  User,
  Wrench,
  X,
  Zap,
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
// Plugins consume the app icon vocabulary (<Icon name="search" size={14} />)
// instead of depending on lucide component names directly.

export type IconName =
  | "search"
  | "plus"
  | "chat"
  | "folder"
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
  | "lightning"
  | "bug"
  | "shield"
  | "loop"
  | "copy"
  | "chevron-up"
  | "chevron-down"
  | "chevron-right"
  | "panel-r"
  | "arrow-down"
  | "trash"
  | "alert"
  | "eye"
  | "file-plus"
  | "folder-search"
  | "download"
  | "bot"
  | "question"
  | "star";

// Mapping from our project's icon vocabulary to lucide components.
// Names on the left are the project's IconName tokens used at every
// callsite; names on the right are the Feather/Lucide-canonical
// equivalents we render under the hood.
const ICON_MAP: Record<IconName, LucideIcon> = {
  search: Search,
  plus: Plus,
  chat: MessageSquare,
  folder: Folder,
  code: Code,
  terminal: Terminal,
  file: File,
  filetext: FileText,
  send: Send,
  "send-arrow": Send,
  stop: Square,
  play: Play,
  pause: Pause,
  settings: Settings,
  sun: Sun,
  moon: Moon,
  share: Share2,
  more: MoreHorizontal,
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
  lightning: Zap,
  bug: Bug,
  shield: Shield,
  loop: Repeat,
  copy: Copy,
  "chevron-up": ChevronUp,
  "chevron-down": ChevronDown,
  "chevron-right": ChevronRight,
  "arrow-down": ArrowDown,
  trash: Trash2,
  alert: AlertTriangle,
  eye: Eye,
  "file-plus": FilePlus,
  "folder-search": FolderSearch,
  download: Download,
  bot: Bot,
  question: MessageCircleQuestion,
  star: Star,
};

interface Props {
  name: IconName;
  size?: number;
  strokeWidth?: number;
  style?: CSSProperties;
  className?: string;
}

export function Icon({ name, size = 16, strokeWidth = 2, style, className }: Props) {
  const Glyph = ICON_MAP[name];
  if (!Glyph) return null;
  return (
    <Glyph
      size={size}
      strokeWidth={strokeWidth}
      style={style}
      className={className}
      aria-hidden="true"
    />
  );
}
