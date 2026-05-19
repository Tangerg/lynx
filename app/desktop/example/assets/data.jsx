// ============================================================
// Sonance Agent — Icons (inline SVGs)
// Lucide-style, 2px stroke, currentColor
// ============================================================

const Icon = ({ name, size = 16, strokeWidth = 2, style }) => {
  const p = { width: size, height: size, viewBox: "0 0 24 24", fill: "none", stroke: "currentColor", strokeWidth, strokeLinecap: "round", strokeLinejoin: "round", style };
  switch (name) {
    case "search": return <svg {...p}><circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/></svg>;
    case "plus": return <svg {...p}><path d="M12 5v14M5 12h14"/></svg>;
    case "chat": return <svg {...p}><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>;
    case "folder": return <svg {...p}><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.7-.9L9.6 3.9A2 2 0 0 0 7.9 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z"/></svg>;
    case "code": return <svg {...p}><path d="m16 18 6-6-6-6M8 6l-6 6 6 6"/></svg>;
    case "terminal": return <svg {...p}><path d="m4 17 6-6-6-6M12 19h8"/></svg>;
    case "file": return <svg {...p}><path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/></svg>;
    case "filetext": return <svg {...p}><path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/><path d="M10 9H8M16 13H8M16 17H8"/></svg>;
    case "send": return <svg {...p} fill="currentColor" stroke="none"><path d="M3 11.5 21 3l-8.5 18-2-8z"/></svg>;
    case "send-arrow": return <svg {...p}><path d="M12 19V5M5 12l7-7 7 7"/></svg>;
    case "stop": return <svg {...p} fill="currentColor" stroke="none"><rect x="6" y="6" width="12" height="12" rx="2"/></svg>;
    case "play": return <svg {...p} fill="currentColor" stroke="none"><path d="M8 5v14l11-7z"/></svg>;
    case "pause": return <svg {...p} fill="currentColor" stroke="none"><rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/></svg>;
    case "settings": return <svg {...p}><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09a1.65 1.65 0 0 0-1-1.51 1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09a1.65 1.65 0 0 0 1.51-1 1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33h0a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82v0a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>;
    case "sun": return <svg {...p}><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>;
    case "moon": return <svg {...p}><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>;
    case "share": return <svg {...p}><circle cx="18" cy="5" r="3"/><circle cx="6" cy="12" r="3"/><circle cx="18" cy="19" r="3"/><path d="m8.59 13.51 6.83 3.98M15.41 6.51 8.59 10.49"/></svg>;
    case "more": return <svg {...p}><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/><circle cx="5" cy="12" r="1"/></svg>;
    case "x": return <svg {...p}><path d="M18 6 6 18M6 6l12 12"/></svg>;
    case "check": return <svg {...p}><path d="M20 6 9 17l-5-5"/></svg>;
    case "branch": return <svg {...p}><line x1="6" y1="3" x2="6" y2="15"/><circle cx="18" cy="6" r="3"/><circle cx="6" cy="18" r="3"/><path d="M18 9a9 9 0 0 1-9 9"/></svg>;
    case "git": return <svg {...p}><circle cx="12" cy="12" r="10"/><path d="M14.5 9.5 9.5 14.5M9.5 9.5 14.5 14.5"/></svg>;
    case "globe": return <svg {...p}><circle cx="12" cy="12" r="10"/><path d="M2 12h20M12 2a15 15 0 0 1 4 10 15 15 0 0 1-4 10 15 15 0 0 1-4-10 15 15 0 0 1 4-10z"/></svg>;
    case "book": return <svg {...p}><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2"/></svg>;
    case "history": return <svg {...p}><path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5M12 7v5l4 2"/></svg>;
    case "tool": return <svg {...p}><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94L9 17.25A2.83 2.83 0 1 1 5 13.4l6.78-6.78a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/></svg>;
    case "sparkle": return <svg {...p}><path d="m12 3-1.9 5.8L4 10l6.1 1.5L12 17l1.9-5.5L20 10l-6.1-1.2z"/></svg>;
    case "edit": return <svg {...p}><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.12 2.12 0 0 1 3 3L12 15l-4 1 1-4z"/></svg>;
    case "paperclip": return <svg {...p}><path d="m21.44 11.05-9.19 9.19a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48"/></svg>;
    case "image": return <svg {...p}><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="9" cy="9" r="2"/><path d="m21 15-5-5L5 21"/></svg>;
    case "command": return <svg {...p}><path d="M18 3a3 3 0 0 0-3 3v12a3 3 0 0 0 3 3 3 3 0 0 0 3-3 3 3 0 0 0-3-3H6a3 3 0 0 0-3 3 3 3 0 0 0 3 3 3 3 0 0 0 3-3V6a3 3 0 0 0-3-3 3 3 0 0 0-3 3 3 3 0 0 0 3 3h12a3 3 0 0 0 3-3 3 3 0 0 0-3-3z"/></svg>;
    case "panel": return <svg {...p}><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M15 3v18"/></svg>;
    case "panel-l": return <svg {...p}><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M9 3v18"/></svg>;
    case "user": return <svg {...p}><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>;
    case "spark": return <svg {...p} fill="currentColor" stroke="none"><path d="m12 2 2.5 7L22 12l-7.5 3L12 22l-2.5-7L2 12l7.5-3z"/></svg>;
    case "skip-back": return <svg {...p} fill="currentColor" stroke="none"><path d="M19 20 9 12l10-8zM5 19V5"/></svg>;
    case "skip-fwd": return <svg {...p} fill="currentColor" stroke="none"><path d="m5 4 10 8-10 8zM19 5v14"/></svg>;
    case "minimize": return <svg {...p}><path d="M4 14h6v6M20 10h-6V4M14 10l7-7M3 21l7-7"/></svg>;
    case "diff": return <svg {...p}><path d="M12 3v18M3 8h6M3 16h6M15 12l3-3 3 3M18 9v6"/></svg>;
    case "list": return <svg {...p}><path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01"/></svg>;
    case "lightning": return <svg {...p} fill="currentColor" stroke="none"><path d="M13 2 3 14h8l-1 8 10-12h-8z"/></svg>;
    case "bug": return <svg {...p}><path d="m8 2 1.88 1.88M14.12 3.88 16 2M9 7.13v-1a3.003 3.003 0 1 1 6 0v1M12 20c-3.3 0-6-2.7-6-6v-3a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v3c0 3.3-2.7 6-6 6M12 20v-9M6.53 9C4.6 8.8 3 7.1 3 5M6 13H2M3 21c0-2.1 1.7-3.9 3.8-4M20.97 5c0 2.1-1.6 3.8-3.5 4M16 13h4M21 21c0-2.1-1.7-3.9-3.8-4"/></svg>;
    case "shield": return <svg {...p}><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10"/></svg>;
    case "loop": return <svg {...p}><path d="m17 2 4 4-4 4M3 11v-1a4 4 0 0 1 4-4h14M7 22l-4-4 4-4M21 13v1a4 4 0 0 1-4 4H3"/></svg>;
    default: return null;
  }
};

// ============================================================
// Mock data: sessions, current conversation, file tree, diffs
// ============================================================

const SESSIONS = [
  { id: "s1", sid: "01.A4F", title: "Refactor auth.ts → Result<T,E>", project: "fern-api", status: "running", time: "now", active: true, model: "Sonnet 4.5", signal: 4 },
  { id: "s2", sid: "01.B7E", title: "Bug: race in WebSocket reconnect logic", project: "fern-api", status: "waiting", time: "3m", model: "Opus 4.1", signal: 3 },
  { id: "s3", sid: "01.C12", title: "Write integration tests for /v2/billing", project: "fern-api", status: "idle", time: "1h", model: "Sonnet 4.5", signal: 2 },
  { id: "s4", sid: "02.F09", title: "Migrate Postgres 15 → 16 on staging", project: "infra", status: "idle", time: "yesterday", model: "Opus 4.1", signal: 1 },
  { id: "s5", sid: "01.D81", title: "Draft RFC: query budget enforcement", project: "fern-api", status: "idle", time: "2d", model: "Sonnet 4.5", signal: 2 },
  { id: "s6", sid: "01.E2F", title: "Replace Stripe webhook handler", project: "fern-api", status: "idle", time: "4d", model: "Haiku 4.5", signal: 1 },
  { id: "s7", sid: "02.G35", title: "Investigate 502s on us-east-1", project: "infra", status: "idle", time: "1w", model: "Sonnet 4.5", signal: 0 },
];

const PROJECTS = [
  { id: "p1", code: "VLT.01", name: "fern-api", branch: "feat/result-type", path: "~/code/fern-api", active: true },
  { id: "p2", code: "VLT.02", name: "infra", branch: "main", path: "~/code/infra" },
  { id: "p3", code: "VLT.03", name: "marketing-site", branch: "main", path: "~/code/marketing-site" },
];

const MODELS = [
  { id: "sonnet45", name: "Claude Sonnet 4.5", sub: "Balanced · 200k ctx", initial: "S" },
  { id: "opus41", name: "Claude Opus 4.1", sub: "Deep reasoning · 200k", initial: "O" },
  { id: "haiku45", name: "Claude Haiku 4.5", sub: "Fast · 200k", initial: "H" },
  { id: "gpt5", name: "GPT-5", sub: "256k ctx", initial: "G" },
];

const PLAN = [
  { id: 1, pid: "T-001", status: "done", text: "Read src/api/auth.ts and identify error-throwing call sites" },
  { id: 2, pid: "T-002", status: "done", text: "Grep callers of login(), refresh(), and verifyToken() across the repo" },
  { id: 3, pid: "T-003", status: "done", text: "Introduce Result<T,E> type in src/lib/result.ts with helpers ok() and err()" },
  { id: 4, pid: "T-004", status: "done", text: "Refactor auth.ts to return Result instead of throwing" },
  { id: 5, pid: "T-005", status: "doing", text: "Update 7 call sites to handle the new return shape" },
  { id: 6, pid: "T-006", status: "todo", text: "Run pnpm test and pnpm typecheck" },
  { id: 7, pid: "T-007", status: "todo", text: "Update CHANGELOG.md and open a draft PR" },
];

const TOOL_CALLS = [
  { id: "t1", pid: "TX/04A1", fn: "read_file", args: "src/api/auth.ts", status: "ok", duration: "12ms", lines: 247, bytes: "8.4KB" },
  { id: "t2", pid: "TX/04A2", fn: "grep", args: "\"login\\\\(|refresh\\\\(|verifyToken\\\\(\"", status: "ok", duration: "34ms", lines: 23, hits: 14 },
  { id: "tw", pid: "TX/04A3", fn: "web_search", args: "Result<T,E> type pattern TypeScript best practices", status: "ok", duration: "1.2s", lines: 0, hits: 3 },
  { id: "t3", pid: "TX/04A4", fn: "write_file", args: "src/lib/result.ts", status: "ok", duration: "8ms", added: 38, removed: 0 },
  { id: "t4", pid: "TX/04A5", fn: "edit_file", args: "src/api/auth.ts", status: "ok", duration: "11ms", added: 47, removed: 31, selected: true },
  { id: "t5", pid: "TX/04A6", fn: "bash", args: "pnpm typecheck", status: "ok", duration: "2.4s", lines: 0 },
  { id: "t6", pid: "TX/04A7", fn: "edit_file", args: "src/api/billing.ts", status: "ok", duration: "9ms", added: 3, removed: 3 },
  { id: "t7", pid: "TX/04A8", fn: "bash", args: "pnpm typecheck", status: "running", duration: "LIVE" },
];

// Web-search citation results for tool tw
const SEARCH_RESULTS = [
  { domain: "matklad.github.io", title: "Result vs. Exceptions — why TypeScript adopted both", time: "2024", snippet: "The Result pattern shines for recoverable errors at boundaries; throwing remains idiomatic for programmer bugs." },
  { domain: "effect.website", title: "Effect — Building blocks for typed errors", time: "2025", snippet: "Effect's tagged Result and Either generalize the pattern, but plain Result<T, E> is enough for most service-level code." },
  { domain: "github.com/supermacro/neverthrow", title: "neverthrow — Type-safe errors for TS", time: "★ 5.2k", snippet: "Lightweight Result type with combinators (map, andThen, mapErr). The most popular pure-TS implementation." },
];

// The code block agent proposes before writing result.ts
const PROPOSED_CODE = `export type Ok<T>  = { readonly ok: true;  readonly value: T };
export type Err<E> = { readonly ok: false; readonly error: E };
export type Result<T, E> = Ok<T> | Err<E>;

export const ok  = <T>(value: T): Ok<T>  => ({ ok: true,  value });
export const err = <E>(error: E): Err<E> => ({ ok: false, error });

export const map = <T, U, E>(r: Result<T, E>, f: (t: T) => U): Result<U, E> =>
  r.ok ? ok(f(r.value)) : r;

export const unwrapOr = <T, E>(r: Result<T, E>, fallback: T): T =>
  r.ok ? r.value : fallback;`;

// Diff content for src/api/auth.ts
const DIFF = [
  { type: "hunk", text: "@@ -42,18 +42,28 @@ export class AuthClient {" },
  { type: "ctx", l: 42, r: 42, code: "  private session: Session | null = null;" },
  { type: "ctx", l: 43, r: 43, code: "" },
  { type: "del", l: 44, code: "  async login(creds: Credentials): Promise<Session> {" },
  { type: "add", r: 44, code: "  async login(creds: Credentials): Promise<Result<Session, AuthError>> {" },
  { type: "ctx", l: 45, r: 45, code: "    const res = await this.http.post('/v2/auth/login', creds);" },
  { type: "del", l: 46, code: "    if (!res.ok) throw new AuthError('LOGIN_FAILED', res.status);" },
  { type: "add", r: 46, code: "    if (!res.ok) return err(new AuthError('LOGIN_FAILED', res.status));" },
  { type: "del", l: 47, code: "    const data = await res.json();" },
  { type: "del", l: 48, code: "    if (!isSession(data)) throw new AuthError('BAD_SHAPE');" },
  { type: "add", r: 47, code: "    const data = await res.json();" },
  { type: "add", r: 48, code: "    if (!isSession(data)) return err(new AuthError('BAD_SHAPE'));" },
  { type: "ctx", l: 49, r: 49, code: "    this.session = data;" },
  { type: "del", l: 50, code: "    return data;" },
  { type: "add", r: 50, code: "    return ok(data);" },
  { type: "ctx", l: 51, r: 51, code: "  }" },
  { type: "ctx", l: 52, r: 52, code: "" },
  { type: "hunk", text: "@@ -71,12 +81,18 @@ export class AuthClient {" },
  { type: "del", l: 71, code: "  async refresh(): Promise<Session> {" },
  { type: "add", r: 81, code: "  async refresh(): Promise<Result<Session, AuthError>> {" },
  { type: "ctx", l: 72, r: 82, code: "    if (!this.session) {" },
  { type: "del", l: 73, code: "      throw new AuthError('NO_SESSION');" },
  { type: "add", r: 83, code: "      return err(new AuthError('NO_SESSION'));" },
  { type: "ctx", l: 74, r: 84, code: "    }" },
];

const FILES_CHANGED = [
  { path: "src/api/auth.ts", change: "mod", added: 47, removed: 31, active: true },
  { path: "src/lib/result.ts", change: "add", added: 38, removed: 0 },
  { path: "src/api/__tests__/auth.test.ts", change: "mod", added: 14, removed: 8 },
  { path: "src/api/billing.ts", change: "mod", added: 3, removed: 3 },
  { path: "src/api/users.ts", change: "mod", added: 5, removed: 5 },
  { path: "src/api/index.ts", change: "mod", added: 2, removed: 0 },
  { path: "src/components/LoginForm.tsx", change: "mod", added: 9, removed: 4 },
];

// Terminal output for the running bash call
const TERM_LINES = [
  { kind: "prompt", text: "$ " }, { kind: "cmd", text: "pnpm typecheck\n" },
  { kind: "mute", text: "Lockfile is up to date, resolution step is skipped\n" },
  { kind: "mute", text: "Already up to date.\n\n" },
  { kind: "out", text: "> fern-api@2.14.0 typecheck /workspace/fern-api\n" },
  { kind: "out", text: "> tsc --noEmit\n\n" },
  { kind: "out", text: "src/api/billing.ts:142:18 - " }, { kind: "err", text: "error TS2345: " },
  { kind: "out", text: "Argument of type 'Session' is not assignable to parameter of type\n  'Result<Session, AuthError>'.\n\n" },
  { kind: "mute", text: "142   const charge = await stripe.charges.create(session);\n" },
  { kind: "mute", text: "                                                 ~~~~~~~\n\n" },
  { kind: "out", text: "src/components/LoginForm.tsx:78:12 - " }, { kind: "warn", text: "warning TS6133: " },
  { kind: "out", text: "'session' is declared but its value is never read.\n\n" },
  { kind: "mute", text: "Found 1 error, 1 warning in 2 files. Watching for changes...\n" },
];

// Conversation messages (initial state)
const INITIAL_MESSAGES = [
  {
    id: "m1", role: "user", who: "You", time: "2:14 PM",
    content: [
      { type: "text", text: "Refactor `src/api/auth.ts` to return `Result<T, E>` instead of throwing. Update all call sites and make sure typecheck passes. Don't change the public auth flow." }
    ]
  },
  {
    id: "m2", role: "agent", who: "Sonnet 4.5", time: "2:14 PM",
    content: [
      { type: "text", text: "I'll handle this in 7 steps. Reading the file first to see the current shape, then I'll introduce a `Result` type and refactor outward." },
      { type: "plan" },
      { type: "text", text: "Pulling in the file and finding everywhere these functions are called." },
      { type: "tool", toolId: "t1" },
      { type: "tool", toolId: "t2" },
      { type: "text", text: "Found **14 call sites across 7 files**. Before writing, let me check how teams typically shape `Result<T,E>` in TypeScript — I want to keep this idiomatic." },
      { type: "tool", toolId: "tw" },
      { type: "search", toolId: "tw" },
      { type: "text", text: "Going with a minimal **discriminated-union** flavor — same shape as **neverthrow** but without the dependency. Here's what I'll add to `src/lib/result.ts`:" },
      { type: "code", lang: "typescript", file: "src/lib/result.ts", text: PROPOSED_CODE },
      { type: "tool", toolId: "t3" },
      { type: "text", text: "Now refactoring `auth.ts`. Three methods change shape: `login`, `refresh`, `verifyToken`. None of the call signatures change — just the return type." },
      { type: "tool", toolId: "t4" },
      { type: "text", text: "Refactor applied — **+47 / −31**. Running typecheck to find any callers I haven't migrated:" },
      { type: "tool", toolId: "t5" },
      { type: "text", text: "Got one error in `billing.ts:142` — a caller still expects `Session` directly. Fixing:" },
      { type: "tool", toolId: "t6" },
      { type: "approval", text: "Run integration tests for the auth + billing slice", command: "pnpm test --filter=auth --filter=billing", reason: "Tests touch the Stripe sandbox API. Output is logged but no charges are made." },
      { type: "tool", toolId: "t7" },
    ]
  },
  {
    id: "m3", role: "user", who: "You", time: "2:18 PM",
    content: [
      { type: "text", text: "Approved. Once tests pass, also bump `CHANGELOG.md` under **Unreleased → Changed**." }
    ]
  },
  {
    id: "m4", role: "agent", who: "Sonnet 4.5", time: "2:18 PM",
    content: [
      { type: "stream", text: "Got it. Watching the test run; will queue the changelog edit right after the green check." },
    ]
  },
];

// MCP / external tool integrations (for the Tools inspector tab)
const MCP_SERVERS = [
  { id: "fs", name: "Filesystem", desc: "Read & write files in workspace", tools: 6, status: "active", icon: "folder" },
  { id: "git", name: "Git", desc: "Branches, commits, diffs, blame", tools: 12, status: "active", icon: "branch" },
  { id: "sh", name: "Shell", desc: "Execute commands · pnpm / cargo / go", tools: 4, status: "active", icon: "terminal" },
  { id: "web", name: "Web Search", desc: "Brave Search · 1000 queries / mo", tools: 2, status: "active", icon: "globe" },
  { id: "lin", name: "Linear", desc: "fern-api workspace · 47 issues", tools: 8, status: "active", icon: "list" },
  { id: "gh", name: "GitHub", desc: "Repos · PRs · Issues · Actions", tools: 14, status: "active", icon: "git" },
  { id: "pg", name: "Postgres", desc: "Read-only · staging", tools: 5, status: "idle", icon: "tool" },
  { id: "slack", name: "Slack", desc: "Reconnect required", tools: 0, status: "error", icon: "chat" },
];

Object.assign(window, { Icon, SESSIONS, PROJECTS, MODELS, PLAN, TOOL_CALLS, DIFF, FILES_CHANGED, TERM_LINES, INITIAL_MESSAGES, SEARCH_RESULTS, PROPOSED_CODE, MCP_SERVERS });
