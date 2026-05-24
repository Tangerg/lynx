// Scripted demo data — the prose and tool calls a "real" agent would emit.
// Lives apart from the protocol layer so it can be swapped freely.

export const ACTIVITY_LINES = [
  "pnpm typecheck · src/api/billing.ts",
  "pnpm typecheck · src/components/LoginForm.tsx",
  "pnpm typecheck · waiting for tsc watcher",
  "Reading src/api/billing.ts:142",
  "Scanning result of pnpm test --filter=auth",
];

export const PROPOSED_CODE = `export type Ok<T>  = { readonly ok: true;  readonly value: T };
export type Err<E> = { readonly ok: false; readonly error: E };
export type Result<T, E> = Ok<T> | Err<E>;

export const ok  = <T>(value: T): Ok<T>  => ({ ok: true,  value });
export const err = <E>(error: E): Err<E> => ({ ok: false, error });

export const map = <T, U, E>(r: Result<T, E>, f: (t: T) => U): Result<U, E> =>
  r.ok ? ok(f(r.value)) : r;

export const unwrapOr = <T, E>(r: Result<T, E>, fallback: T): T =>
  r.ok ? r.value : fallback;`;

export const PLAN_ITEMS = [
  {
    id: 1,
    pid: "T-001",
    status: "done" as const,
    text: "Read src/api/auth.ts and identify error-throwing call sites",
  },
  {
    id: 2,
    pid: "T-002",
    status: "done" as const,
    text: "Grep callers of login(), refresh(), and verifyToken() across the repo",
  },
  {
    id: 3,
    pid: "T-003",
    status: "done" as const,
    text: "Introduce Result<T,E> type in src/lib/result.ts with helpers ok() and err()",
  },
  {
    id: 4,
    pid: "T-004",
    status: "done" as const,
    text: "Refactor auth.ts to return Result instead of throwing",
  },
  {
    id: 5,
    pid: "T-005",
    status: "doing" as const,
    text: "Update 7 call sites to handle the new return shape",
  },
  { id: 6, pid: "T-006", status: "todo" as const, text: "Run pnpm test and pnpm typecheck" },
  { id: 7, pid: "T-007", status: "todo" as const, text: "Update CHANGELOG.md and open a draft PR" },
];

export const SEARCH_RESULTS = [
  {
    domain: "matklad.github.io",
    title: "Result vs. Exceptions — why TypeScript adopted both",
    time: "2024",
    snippet:
      "The Result pattern shines for recoverable errors at boundaries; throwing remains idiomatic for programmer bugs.",
  },
  {
    domain: "effect.website",
    title: "Effect — Building blocks for typed errors",
    time: "2025",
    snippet:
      "Effect's tagged Result and Either generalize the pattern, but plain Result<T, E> is enough for most service-level code.",
  },
  {
    domain: "github.com/supermacro/neverthrow",
    title: "neverthrow — Type-safe errors for TS",
    time: "★ 5.2k",
    snippet:
      "Lightweight Result type with combinators (map, andThen, mapErr). The most popular pure-TS implementation.",
  },
];

export const APPROVAL = {
  text: "Run integration tests for the auth + billing slice",
  command: "pnpm test --filter=auth --filter=billing",
  reason: "Tests touch the Stripe sandbox API. Output is logged but no charges are made.",
};

// Scripted tool calls — id, fn name, arg string, and summary fields surfaced
// on TOOL_CALL_END.
export const TOOL_SCRIPT = [
  { id: "t1", fn: "read_file", args: "src/api/auth.ts", durationMs: 12, lines: 247 },
  { id: "t2", fn: "grep", args: '"login\\(|refresh\\(|verifyToken\\("', durationMs: 34, hits: 14 },
  {
    id: "tw",
    fn: "web_search",
    args: "Result<T,E> type pattern TypeScript best practices",
    durationMs: 1200,
    hits: 3,
  },
  { id: "t3", fn: "write_file", args: "src/lib/result.ts", durationMs: 8, added: 38, removed: 0 },
  { id: "t4", fn: "edit_file", args: "src/api/auth.ts", durationMs: 11, added: 47, removed: 31 },
  { id: "t5", fn: "bash", args: "pnpm typecheck", durationMs: 2400 },
  { id: "t6", fn: "edit_file", args: "src/api/billing.ts", durationMs: 9, added: 3, removed: 3 },
];

export const USER_PROMPT =
  "Refactor `src/api/auth.ts` to return `Result<T, E>` instead of throwing. Update all call sites and make sure typecheck passes. Don't change the public auth flow.";

export const INTRO_TEXT =
  "I'll handle this in 7 steps. Reading the file first to see the current shape, then I'll introduce a `Result` type and refactor outward.";

export const POST_PLAN_TEXT =
  "Pulling in the file and finding everywhere these functions are called.";

export const POST_GREP_TEXT =
  "Found **14 call sites across 7 files**. Before writing, let me check how teams typically shape `Result<T,E>` in TypeScript — I want to keep this idiomatic.";

export const POST_SEARCH_TEXT =
  "Going with a minimal **discriminated-union** flavor — same shape as **neverthrow** but without the dependency. Here's what I'll add to `src/lib/result.ts`:";

export const POST_WRITE_TEXT =
  "Now refactoring `auth.ts`. Three methods change shape: `login`, `refresh`, `verifyToken`. None of the call signatures change — just the return type.";

export const POST_EDIT_TEXT =
  "Refactor applied — **+47 / −31**. Running typecheck to find any callers I haven't migrated:";

export const POST_TYPECHECK_TEXT =
  "Got one error in `billing.ts:142` — a caller still expects `Session` directly. Fixing:";
