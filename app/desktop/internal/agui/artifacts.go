package agui

import "time"

// Static artifacts served via REST. Kept in one place; the HTTP handlers in
// rest.go just marshal these to JSON. In production these would back onto
// real workspace state.

// ----- Sessions ------------------------------------------------------------

type Session struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"` // running | waiting | idle
	Model  string `json:"model"`
	// RFC3339 timestamp. Frontend formats with dayjs's relativeTime
	// so the displayed label stays compact + consistent (now / 3m /
	// 1h / 1d / 2w / 1mo) — the previous mix of hand-rolled strings
	// ("yesterday" alongside "3m" / "2d") was inconsistent.
	Time string `json:"time"`
}

// sessionOffsets — base offsets per session id, expressed as durations
// before "now". Re-evaluated on every request so the frontend always
// gets fresh "X ago" data without us having to bake in absolute dates.
var sessionOffsets = []struct {
	id, title, status, model string
	ago                      time.Duration
}{
	{"s1", "Refactor auth.ts → Result<T,E>", "running", "Sonnet 4.5", 0},
	{"s2", "番茄牛肉意面：家常版菜谱", "idle", "Sonnet 4.5", 3 * time.Minute},
	{"s3", "上海周末两日游 · 行程规划", "idle", "Opus 4.1", 1 * time.Hour},
	{"s4", "发布会开场白润色", "idle", "Sonnet 4.5", 24 * time.Hour},
	{"s5", "光合作用：讲给小学生听", "idle", "Haiku 4.5", 2 * 24 * time.Hour},
	{"s6", "用户登录流程设计（含 OAuth）", "idle", "Sonnet 4.5", 4 * 24 * time.Hour},
	{"s7", "国内云厂商横向对比", "idle", "Opus 4.1", 7 * 24 * time.Hour},
}

// makeSessions builds the session list with timestamps relative to the
// current wall clock. Called by the /sessions handler on each request.
func makeSessions() []Session {
	now := time.Now()
	out := make([]Session, len(sessionOffsets))
	for i, s := range sessionOffsets {
		out[i] = Session{
			ID:     s.id,
			Title:  s.title,
			Status: s.status,
			Model:  s.model,
			Time:   now.Add(-s.ago).Format(time.RFC3339),
		}
	}
	return out
}

// ----- Projects ------------------------------------------------------------

type Project struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Branch string `json:"branch"`
	Active bool   `json:"active,omitempty"`
}

var projects = []Project{
	{"p1", "fern-api", "feat/result-type", true},
	{"p2", "infra", "main", false},
	{"p3", "marketing-site", "main", false},
}

// ----- Terminal / Diff / Grep / FileHead (tool artifacts) ------------------

type TermLine struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

var termLines = []TermLine{
	{"prompt", "$ "}, {"cmd", "pnpm typecheck\n"},
	{"mute", "Lockfile is up to date, resolution step is skipped\n"},
	{"mute", "Already up to date.\n\n"},
	{"out", "> fern-api@2.14.0 typecheck /workspace/fern-api\n"},
	{"out", "> tsc --noEmit\n\n"},
	{"out", "src/api/billing.ts:142:18 - "}, {"err", "error TS2345: "},
	{"out", "Argument of type 'Session' is not assignable to parameter of type\n  'Result<Session, AuthError>'.\n\n"},
	{"mute", "142   const charge = await stripe.charges.create(session);\n"},
	{"mute", "                                                 ~~~~~~~\n\n"},
	{"out", "src/components/LoginForm.tsx:78:12 - "}, {"warn", "warning TS6133: "},
	{"out", "'session' is declared but its value is never read.\n\n"},
	{"mute", "Found 1 error, 1 warning in 2 files. Watching for changes...\n"},
}

// DiffRow uses a discriminated shape — Type plus either L (left line), R
// (right line), Code, or Text. We embed all of them and let the client pick.
type DiffRow struct {
	Type string `json:"type"`           // hunk | ctx | add | del
	Text string `json:"text,omitempty"` // hunk text
	L    int    `json:"l,omitempty"`    // left line number
	R    int    `json:"r,omitempty"`    // right line number
	Code string `json:"code,omitempty"`
}

var diff = []DiffRow{
	{Type: "hunk", Text: "@@ -42,18 +42,28 @@ export class AuthClient {"},
	{Type: "ctx", L: 42, R: 42, Code: "  private session: Session | null = null;"},
	{Type: "ctx", L: 43, R: 43, Code: ""},
	{Type: "del", L: 44, Code: "  async login(creds: Credentials): Promise<Session> {"},
	{Type: "add", R: 44, Code: "  async login(creds: Credentials): Promise<Result<Session, AuthError>> {"},
	{Type: "ctx", L: 45, R: 45, Code: "    const res = await this.http.post('/v2/auth/login', creds);"},
	{Type: "del", L: 46, Code: "    if (!res.ok) throw new AuthError('LOGIN_FAILED', res.status);"},
	{Type: "add", R: 46, Code: "    if (!res.ok) return err(new AuthError('LOGIN_FAILED', res.status));"},
	{Type: "del", L: 47, Code: "    const data = await res.json();"},
	{Type: "del", L: 48, Code: "    if (!isSession(data)) throw new AuthError('BAD_SHAPE');"},
	{Type: "add", R: 47, Code: "    const data = await res.json();"},
	{Type: "add", R: 48, Code: "    if (!isSession(data)) return err(new AuthError('BAD_SHAPE'));"},
	{Type: "ctx", L: 49, R: 49, Code: "    this.session = data;"},
	{Type: "del", L: 50, Code: "    return data;"},
	{Type: "add", R: 50, Code: "    return ok(data);"},
	{Type: "ctx", L: 51, R: 51, Code: "  }"},
	{Type: "ctx", L: 52, R: 52, Code: ""},
	{Type: "hunk", Text: "@@ -71,12 +81,18 @@ export class AuthClient {"},
	{Type: "del", L: 71, Code: "  async refresh(): Promise<Session> {"},
	{Type: "add", R: 81, Code: "  async refresh(): Promise<Result<Session, AuthError>> {"},
	{Type: "ctx", L: 72, R: 82, Code: "    if (!this.session) {"},
	{Type: "del", L: 73, Code: "      throw new AuthError('NO_SESSION');"},
	{Type: "add", R: 83, Code: "      return err(new AuthError('NO_SESSION'));"},
	{Type: "ctx", L: 74, R: 84, Code: "    }"},
}

type GrepMatch struct {
	Path  string `json:"path"`
	Match string `json:"match"`
}

type GrepResult struct {
	Matches []GrepMatch `json:"matches"`
	Total   int         `json:"total"`
}

var grep = GrepResult{
	Matches: []GrepMatch{
		{"src/api/auth.ts:44", "async login(creds: Credentials)"},
		{"src/api/auth.ts:71", "async refresh(): Promise<Session>"},
		{"src/api/users.ts:18", "await client.login(credentials)"},
		{"src/api/billing.ts:142", "const session = await refresh()"},
	},
	Total: 14,
}

type FileLine struct {
	Ln    string `json:"ln"`
	Code  string `json:"code"`
	Muted bool   `json:"muted,omitempty"`
}

var fileHead = []FileLine{
	{Ln: "1", Code: `<span class="t-kw">import</span> { Credentials, Session } <span class="t-kw">from</span> <span class="t-str">'./types'</span>;`},
	{Ln: "2", Code: `<span class="t-kw">import</span> { HttpClient } <span class="t-kw">from</span> <span class="t-str">'../http'</span>;`},
	{Ln: "3", Code: ""},
	{Ln: "4", Code: `<span class="t-kw">export class</span> <span class="t-fn">AuthClient</span> {`},
	{Ln: "5", Code: `  <span class="t-kw">constructor</span>(<span class="t-kw">private</span> http: HttpClient) {}`},
	{Ln: "···", Code: "242 more lines", Muted: true},
}

// ----- Files changed ------------------------------------------------------

type FileChange struct {
	Path    string `json:"path"`
	Change  string `json:"change"` // add | mod | del
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
}

var filesChanged = []FileChange{
	{"src/api/auth.ts", "mod", 47, 31},
	{"src/lib/result.ts", "add", 38, 0},
	{"src/api/__tests__/auth.test.ts", "mod", 14, 8},
	{"src/api/billing.ts", "mod", 3, 3},
	{"src/api/users.ts", "mod", 5, 5},
	{"src/api/index.ts", "mod", 2, 0},
	{"src/components/LoginForm.tsx", "mod", 9, 4},
}

// ----- MCP servers --------------------------------------------------------

type MCPServer struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Desc   string `json:"desc"`
	Tools  int    `json:"tools"`
	Status string `json:"status"`
	Icon   string `json:"icon"`
}

var mcpServers = []MCPServer{
	{"fs", "Filesystem", "Read & write files in workspace", 6, "active", "folder"},
	{"git", "Git", "Branches, commits, diffs, blame", 12, "active", "branch"},
	{"sh", "Shell", "Execute commands · pnpm / cargo / go", 4, "active", "terminal"},
	{"web", "Web Search", "Brave Search · 1000 queries / mo", 2, "active", "globe"},
	{"lin", "Linear", "fern-api workspace · 47 issues", 8, "active", "list"},
	{"gh", "GitHub", "Repos · PRs · Issues · Actions", 14, "active", "git"},
	{"pg", "Postgres", "Read-only · staging", 5, "idle", "tool"},
	{"slack", "Slack", "Reconnect required", 0, "error", "chat"},
}
