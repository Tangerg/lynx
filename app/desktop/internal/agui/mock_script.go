package agui

// Static script content — mirrors frontend/src/protocol/agui/mockScript.ts so
// the demo conversation looks identical whether the mock runs in JS or Go.

var activityLines = []string{
	"pnpm typecheck · src/api/billing.ts",
	"pnpm typecheck · src/components/LoginForm.tsx",
	"pnpm typecheck · waiting for tsc watcher",
	"Reading src/api/billing.ts:142",
	"Scanning result of pnpm test --filter=auth",
	"Resolving symbols in src/lib/result.ts",
	"Compiling 47 TypeScript modules",
	"Linking type declarations from @types/node",
	"Checking exhaustiveness of Result discriminants",
	"Waiting for pnpm test --filter=auth to settle",
	"Loading fixtures from tests/fixtures/auth.json",
	"Stripe sandbox: POST /v1/payment_intents",
	"Stripe sandbox: GET /v1/customers (mock)",
	"vitest · auth.spec.ts · 12/18 passing",
	"vitest · billing.spec.ts · resolving Stripe mocks",
	"Building dependency graph for src/api/**/*.ts",
	"Reading tsconfig.json compiler options",
	"Watching src/lib/result.ts for changes",
}

const proposedCode = `export type Ok<T>  = { readonly ok: true;  readonly value: T };
export type Err<E> = { readonly ok: false; readonly error: E };
export type Result<T, E> = Ok<T> | Err<E>;

export const ok  = <T>(value: T): Ok<T>  => ({ ok: true,  value });
export const err = <E>(error: E): Err<E> => ({ ok: false, error });

export const map = <T, U, E>(r: Result<T, E>, f: (t: T) => U): Result<U, E> =>
  r.ok ? ok(f(r.value)) : r;

export const mapErr = <T, E, F>(r: Result<T, E>, f: (e: E) => F): Result<T, F> =>
  r.ok ? r : err(f(r.error));

export const andThen = <T, U, E>(r: Result<T, E>, f: (t: T) => Result<U, E>): Result<U, E> =>
  r.ok ? f(r.value) : r;

export const unwrapOr = <T, E>(r: Result<T, E>, fallback: T): T =>
  r.ok ? r.value : fallback;

export const isOk = <T, E>(r: Result<T, E>): r is Ok<T> => r.ok;
export const isErr = <T, E>(r: Result<T, E>): r is Err<E> => !r.ok;`

type planItem struct {
	ID     int    `json:"id"`
	PID    string `json:"pid"`
	Status string `json:"status"`
	Text   string `json:"text"`
}

var planItems = []planItem{
	{1, "T-001", "done", "Read src/api/auth.ts and identify error-throwing call sites"},
	{2, "T-002", "done", "Grep callers of login(), refresh(), and verifyToken() across the repo"},
	{3, "T-003", "done", "Introduce Result<T,E> type in src/lib/result.ts with helpers ok() and err()"},
	{4, "T-004", "done", "Refactor auth.ts to return Result instead of throwing"},
	{5, "T-005", "doing", "Update 7 call sites to handle the new return shape"},
	{6, "T-006", "todo", "Run pnpm test and pnpm typecheck"},
	{7, "T-007", "todo", "Update CHANGELOG.md and open a draft PR"},
}

type searchResult struct {
	Domain  string `json:"domain"`
	Title   string `json:"title"`
	Time    string `json:"time"`
	Snippet string `json:"snippet"`
}

var searchResults = []searchResult{
	{"matklad.github.io", "Result vs. Exceptions — why TypeScript adopted both", "2024",
		"The Result pattern shines for recoverable errors at boundaries; throwing remains idiomatic for programmer bugs."},
	{"effect.website", "Effect — Building blocks for typed errors", "2025",
		"Effect's tagged Result and Either generalize the pattern, but plain Result<T, E> is enough for most service-level code."},
	{"github.com/supermacro/neverthrow", "neverthrow — Type-safe errors for TS", "★ 5.2k",
		"Lightweight Result type with combinators (map, andThen, mapErr). The most popular pure-TS implementation."},
	{"blog.cleancoder.com", "When to throw vs. when to return", "2023",
		"Recoverable conditions belong in the type system; truly exceptional ones (OOM, panic) belong in the runtime."},
}

type toolSpec struct {
	ID         string
	Fn         string
	Args       string
	DurationMs int
	Added      *int
	Removed    *int
	Hits       *int
	Lines      *int
}

var toolScript = []toolSpec{
	{"t1", "read_file", "src/api/auth.ts", 12, nil, nil, nil, IntPtr(247)},
	{"t2", "grep", `"login\(|refresh\(|verifyToken\("`, 34, nil, nil, IntPtr(14), nil},
	{"tw", "web_search", "Result<T,E> type pattern TypeScript best practices 2026", 1200, nil, nil, IntPtr(4), nil},
	{"t3", "write_file", "src/lib/result.ts", 8, IntPtr(38), IntPtr(0), nil, nil},
	{"t4", "edit_file", "src/api/auth.ts", 11, IntPtr(47), IntPtr(31), nil, nil},
	{"t5", "bash", "pnpm typecheck", 2400, nil, nil, nil, nil},
	{"t6", "edit_file", "src/api/billing.ts", 9, IntPtr(3), IntPtr(3), nil, nil},
	// Extra steps after the approval (CHANGELOG + draft PR).
	{"t8", "read_file", "CHANGELOG.md", 6, nil, nil, nil, IntPtr(184)},
	{"t9", "edit_file", "CHANGELOG.md", 14, IntPtr(8), IntPtr(0), nil, nil},
}

const (
	userPrompt = "Refactor `src/api/auth.ts` to return `Result<T, E>` instead of throwing. " +
		"Update all call sites and make sure typecheck passes. Don't change the public auth flow."

	introText = "## Plan: refactor `auth.ts` to return `Result<T, E>`\n\n" +
		"I'll work through this in **7 steps**. The high-level idea:\n\n" +
		"- Read `src/api/auth.ts` to see which functions throw\n" +
		"- Grep every caller of `login`, `refresh`, `verifyToken` across the repo\n" +
		"- Introduce a minimal `Result<T, E>` discriminated union in `src/lib/result.ts`\n" +
		"- Migrate `auth.ts` to return the new type — public signatures stay identical\n" +
		"- Walk each call site and swap `try/catch` for `isOk` / `isErr`\n" +
		"- Run `pnpm typecheck` and the auth integration suite\n" +
		"- Update `CHANGELOG.md` and open a draft PR\n\n" +
		"Starting with the file now."

	postPlanText = "Reading the file now and tracing every place these three functions get called. " +
		"If the call sites are mostly inside one or two files this is going to be a tight diff; " +
		"if they're spread out we'll need a more careful rollout — I'll know in a second."

	postGrepText = "OK — **14 call sites across 7 files**. Moderate spread, nothing dramatic. Before " +
		"I commit to a specific `Result` shape, let me skim what the community is doing in 2026. " +
		"I want this to look idiomatic to anyone who lands in this file later, not bespoke. " +
		"Pulling a quick web search:"

	postSearchText = "Reading those four, the consensus is clear: a minimal **discriminated-union** " +
		"flavor is the right call. `neverthrow` is the most popular pure-TS implementation, but " +
		"the dependency isn't worth it for a thirty-line type — and matklad's argument for keeping " +
		"it inline is convincing. Going to mirror neverthrow's shape directly. Here's what I'll " +
		"add to `src/lib/result.ts`:"

	postWriteText = "Result type is in — ok/err/map/mapErr/andThen/unwrapOr/isOk/isErr, plus the " +
		"two type guards for narrowing. Now refactoring `auth.ts` itself. Three methods change " +
		"return shape: `login`, `refresh`, `verifyToken`. The function signatures don't change — " +
		"only the return type — so the diff stays surface-level and the public API is stable."

	// postEditText also doubles as the markdown-feature showcase — pulls
	// in a mermaid diagram, a GFM table, a numbered list, and a
	// blockquote so the new react-markdown + shiki + beautiful-mermaid
	// pipeline gets exercised end-to-end in a single segment.
	postEditText = "**Done.** Refactor applied — **+47 / −31** net. Here's how the new flow looks:\n\n" +
		"```mermaid\n" +
		"sequenceDiagram\n" +
		"    participant Caller\n" +
		"    participant Auth as auth.ts\n" +
		"    participant Store\n" +
		"    Caller->>Auth: login(creds)\n" +
		"    Auth->>Store: verifyCredentials\n" +
		"    Store-->>Auth: Result<Session, StoreError>\n" +
		"    Auth-->>Caller: Result<Session, LoginError>\n" +
		"```\n\n" +
		"The 14 call sites split across these files:\n\n" +
		"| File | Calls | Pattern |\n" +
		"|------|------:|---------|\n" +
		"| `src/api/billing.ts` | 4 | Stripe attach |\n" +
		"| `src/components/LoginForm.tsx` | 3 | Form submit |\n" +
		"| `src/api/admin.ts` | 5 | Audit + bulk |\n" +
		"| `src/api/profile.ts` | 2 | Avatar upload |\n\n" +
		"Each migration follows the same three-step pattern:\n\n" +
		"1. Wrap the call in an `isOk` / `isErr` branch\n" +
		"2. Map the error variant through the existing error reducer\n" +
		"3. Drop the surrounding `try/catch`\n\n" +
		"> Public function signatures stay identical — only the return type changes — so no caller's typedef breaks at the boundary.\n\n" +
		"Running typecheck next:"

	postTypecheckText = "Got exactly one error:\n\n" +
		"```text\n" +
		"src/api/billing.ts:142:23 — Type 'Result<Session, LoginError>' is not assignable to 'Session'.\n" +
		"```\n\n" +
		"The caller still pulls `Session` directly off the return. Patching it to unwrap the " +
		"`Result` first:\n\n" +
		"```diff\n" +
		"- const session = await login(creds);\n" +
		"- attachStripe(session);\n" +
		"+ const result = await login(creds);\n" +
		"+ if (!isOk(result)) return result;\n" +
		"+ attachStripe(result.value);\n" +
		"```"

	postBillingFixText = "Typecheck is clean. **Pre-flight checklist** before opening the PR:\n\n" +
		"- [x] All call sites migrated\n" +
		"- [x] `pnpm typecheck` passes\n" +
		"- [ ] Integration tests against Stripe sandbox\n" +
		"- [ ] `CHANGELOG.md` entry\n" +
		"- [ ] Draft PR\n\n" +
		"The integration suite touches Stripe's sandbox API. It doesn't make real charges, but " +
		"it does hit live endpoints, so this needs your sign-off:"

	postApprovalText = "Got it — kicking off the integration suite now. This is the long-running " +
		"step, so I'll stay attached and surface anything that fails or hangs:"
)

// Custom event names — must match the constants in customEvents.ts on the JS side.
const (
	customPlan          = "lyra.plan"
	customPlanBlock     = "lyra.plan-block"
	customCodeProposal  = "lyra.code-proposal"
	customSearchResults = "lyra.search-results"
	customApproval      = "lyra.approval"
	customTelemetry     = "lyra.telemetry"
)
