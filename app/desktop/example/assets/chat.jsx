// ============================================================
// Sonance Agent — Center chat area
// New: top tab bar (multi-session) · inline tool expansion
// ============================================================

function ChatTopBar({
  tabs, activeId, onSelect, onClose, onNew,
}) {
  return (
    <div className="chat-topbar">
      <div className="topbar-tabs">
        {tabs.map(t => (
          <div
            key={t.id}
            className={`chat-tab ${t.id === activeId ? 'active' : ''}`}
            onClick={() => onSelect(t.id)}
          >
            <span className={`tab-dot ${t.status}`}></span>
            <span className="tab-title" title={t.title}>{t.title}</span>
            <span
              className="tab-close"
              onClick={(e) => { e.stopPropagation(); onClose(t.id); }}
              title="Close"
            ><Icon name="x" size={10} /></span>
          </div>
        ))}
        <button className="tab-new" onClick={onNew} title="New session (⌘N)">
          <Icon name="plus" size={13} />
        </button>
      </div>
    </div>
  );
}

// Status pill — lives below the top bar. Uses the same visual language as
// the composer (rounded surface, padded, soft border) and consolidates
// run-state info so it isn't duplicated above the input.
function StatusPill({ running, step, totalSteps, activity, tokens, ctxPct, cost, onStop }) {
  return (
    <div className="status-wrap">
      <div className={`status-pill ${running ? 'live' : ''}`}>
        {running ? (
          <React.Fragment>
            <span className="sp-state live">
              <span className="sp-dot"></span>
              <span>Step {step}/{totalSteps}</span>
            </span>
            <span className="sp-activity">{activity}</span>
          </React.Fragment>
        ) : (
          <span className="sp-state">
            <span className="sp-dot"></span>
            <span>Idle</span>
          </span>
        )}
        <span className="sp-spacer"></span>
        <span className="sp-stat" title="Context window">
          <span className="sp-ctx-bar"><div style={{width: ctxPct + '%'}}></div></span>
          <span className="sp-ctx-text"><span className="v">{tokens.used}</span> <span className="k">/ {tokens.total}</span></span>
        </span>
        <span className="sp-stat" title="Session cost">
          <span className="k">$</span><span className="v">{cost}</span>
        </span>
        {running && (
          <button className="sp-stop" onClick={onStop} title="Stop (⌘.)">
            <Icon name="stop" size={9} />Stop
          </button>
        )}
      </div>
    </div>
  );
}

// One tool call shown as a card inside the chat — now expandable inline
function ToolCard({ tool, selected, onClick, onOpenInspector, expanded, onToggleExpand }) {
  if (!tool) return null;
  const statusClass = tool.status === 'running' ? 'run' : tool.status === 'ok' ? 'ok' : 'err';
  let iconName = "tool";
  if (tool.fn === "read_file" || tool.fn === "write_file" || tool.fn === "edit_file") iconName = "file";
  if (tool.fn === "grep") iconName = "search";
  if (tool.fn === "bash") iconName = "terminal";
  if (tool.fn === "web_search") iconName = "globe";

  return (
    <div className={`tool-card ${selected ? 'selected' : ''} ${expanded ? 'expanded' : ''}`}>
      <div className="tool-head" onClick={onToggleExpand}>
        <div className={`tool-icon ${statusClass}`}>
          <Icon name={iconName} size={14} />
        </div>
        <div className="tool-name">
          <span className="tool-fn">{tool.fn}</span>
          <span className="tool-args">{tool.args}</span>
        </div>
        <div className="tool-meta">
          {tool.added != null && <span style={{color: 'var(--color-accent)'}}>+{tool.added}</span>}
          {tool.removed != null && <span style={{color: 'var(--color-negative)'}}>−{tool.removed}</span>}
          {tool.hits != null && <span>{tool.hits} matches</span>}
          {tool.lines > 0 && tool.added == null && tool.hits == null && <span>{tool.lines} lines</span>}
          <span>·</span>
          <span>{tool.duration}</span>
        </div>
        <div className={`tool-status ${statusClass}`}>
          {tool.status === 'running' ? 'Running' : tool.status === 'ok' ? 'Done' : 'Failed'}
        </div>
        <button
          className="tool-expand"
          title={expanded ? "Collapse" : "Expand preview"}
          onClick={(e) => { e.stopPropagation(); onToggleExpand(); }}
        >
          <Icon name={expanded ? "minimize" : "more"} size={12} />
        </button>
      </div>
      {expanded && <ToolPreview tool={tool} onOpenInspector={onOpenInspector} />}
    </div>
  );
}

function ToolPreview({ tool, onOpenInspector }) {
  // Compact in-chat preview, varies by tool type.
  if (tool.fn === "bash") {
    return (
      <div className="tool-preview term">
        {window.TERM_LINES.slice(0, 9).map((l, i) => (
          <span key={i} className={l.kind}>{l.text}</span>
        ))}
        <div className="preview-foot">
          <button className="preview-open" onClick={onOpenInspector}>
            Open in inspector <Icon name="share" size={11} />
          </button>
        </div>
      </div>
    );
  }
  if (tool.fn === "edit_file" || tool.fn === "write_file") {
    const slice = window.DIFF.slice(0, 8);
    return (
      <div className="tool-preview">
        <div className="diff-view-mini">
          {slice.map((row, i) => {
            if (row.type === "hunk") return <div key={i} className="diff-hunk-head">{row.text}</div>;
            const cls = row.type === "add" ? "add" : row.type === "del" ? "del" : "ctx";
            const sign = row.type === "add" ? "+" : row.type === "del" ? "−" : " ";
            return (
              <div key={i} className={`diff-line ${cls}`}>
                <span className="sign">{sign}</span>
                <span className="code">{row.code}</span>
              </div>
            );
          })}
        </div>
        <div className="preview-foot">
          <button className="preview-open" onClick={onOpenInspector}>
            View full diff in inspector <Icon name="share" size={11} />
          </button>
        </div>
      </div>
    );
  }
  if (tool.fn === "web_search") {
    return null; // search results render as their own block in the message
  }
  if (tool.fn === "grep") {
    return (
      <div className="tool-preview">
        <div className="grep-preview">
          <div className="grep-line"><span className="path">src/api/auth.ts:44</span> <span className="match">async login(creds: Credentials)</span></div>
          <div className="grep-line"><span className="path">src/api/auth.ts:71</span> <span className="match">async refresh(): Promise&lt;Session&gt;</span></div>
          <div className="grep-line"><span className="path">src/api/users.ts:18</span> <span className="match">await client.login(credentials)</span></div>
          <div className="grep-line"><span className="path">src/api/billing.ts:142</span> <span className="match">const session = await refresh()</span></div>
          <div className="grep-line muted">… 10 more matches</div>
        </div>
        <div className="preview-foot">
          <button className="preview-open" onClick={onOpenInspector}>
            View all matches <Icon name="share" size={11} />
          </button>
        </div>
      </div>
    );
  }
  if (tool.fn === "read_file") {
    return (
      <div className="tool-preview">
        <div className="file-preview">
          <div className="fp-line"><span className="ln">1</span><span className="code"><span className="t-kw">import</span> {'{'} Credentials, Session {'}'} <span className="t-kw">from</span> <span className="t-str">'./types'</span>;</span></div>
          <div className="fp-line"><span className="ln">2</span><span className="code"><span className="t-kw">import</span> {'{'} HttpClient {'}'} <span className="t-kw">from</span> <span className="t-str">'../http'</span>;</span></div>
          <div className="fp-line"><span className="ln">3</span><span className="code"></span></div>
          <div className="fp-line"><span className="ln">4</span><span className="code"><span className="t-kw">export class</span> <span className="t-fn">AuthClient</span> {'{'}</span></div>
          <div className="fp-line"><span className="ln">5</span><span className="code">  <span className="t-kw">constructor</span>(<span className="t-kw">private</span> http: HttpClient) {'{'}{'}'}</span></div>
          <div className="fp-line muted"><span className="ln">···</span><span className="code">242 more lines</span></div>
        </div>
        <div className="preview-foot">
          <button className="preview-open" onClick={onOpenInspector}>
            View full file <Icon name="share" size={11} />
          </button>
        </div>
      </div>
    );
  }
  return null;
}

function PlanBlock({ plan }) {
  return (
    <div className="plan-block">
      <div className="plan-head">
        <Icon name="list" size={12} />
        Plan · {plan.filter(p => p.status === 'done').length} of {plan.length} complete
      </div>
      {plan.map(p => (
        <div key={p.id} className={`plan-item ${p.status}`}>
          <div className="check">
            {p.status === 'done' && <Icon name="check" size={12} strokeWidth={3} />}
          </div>
          <div>{p.text}</div>
        </div>
      ))}
    </div>
  );
}

function MessageBlock({ msg, plan, toolCalls, selectedToolId, onSelectTool, expandedIds, onToggleExpand, onOpenInspector }) {
  const isUser = msg.role === 'user';
  return (
    <div className={`msg ${msg.role}`}>
      <div className="msg-avatar">
        {isUser ? <Icon name="user" size={14} /> : <Icon name="spark" size={14} />}
      </div>
      <div className="msg-body">
        <div className="msg-meta">
          <span className={`who ${msg.role}`}>{msg.who}</span>
          <span>·</span>
          <span>{msg.time}</span>
        </div>
        <div className="msg-content">
          {msg.content.map((part, i) => {
            if (part.type === "text") {
              return <p key={i} dangerouslySetInnerHTML={{__html: renderInline(part.text)}} />;
            }
            if (part.type === "plan") {
              return <PlanBlock key={i} plan={plan} />;
            }
            if (part.type === "tool") {
              const t = toolCalls.find(x => x.id === part.toolId);
              return (
                <ToolCard
                  key={i}
                  tool={t}
                  selected={selectedToolId === part.toolId}
                  onClick={() => onSelectTool(part.toolId)}
                  expanded={expandedIds.has(part.toolId)}
                  onToggleExpand={() => onToggleExpand(part.toolId)}
                  onOpenInspector={() => onOpenInspector(part.toolId)}
                />
              );
            }
            if (part.type === "code") {
              return <CodeBlock key={i} lang={part.lang} file={part.file} text={part.text} />;
            }
            if (part.type === "search") {
              return <SearchResults key={i} results={window.SEARCH_RESULTS} query={part.query} />;
            }
            if (part.type === "approval") {
              return <ApprovalCard key={i} what={part.text} cmd={part.command} reason={part.reason} />;
            }
            if (part.type === "checkpoint") {
              return <Checkpoint key={i} text={part.text} />;
            }
            if (part.type === "stream") {
              return <p key={i} className="streaming"><span dangerouslySetInnerHTML={{__html: renderInline(part.text)}} /><span className="cursor">▌</span></p>;
            }
            return null;
          })}
        </div>
      </div>
    </div>
  );
}

function renderInline(s) {
  return String(s)
    .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
}

function CodeBlock({ lang, file, text }) {
  const [copied, setCopied] = React.useState(false);
  const onCopy = () => {
    try { navigator.clipboard.writeText(text); } catch (e) {}
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };
  return (
    <div className="code-block">
      <div className="code-block-head">
        <span className="lang">{lang || 'text'}</span>
        <span className="fname">{file || ''}</span>
        <button className="copy" onClick={onCopy}>
          <Icon name={copied ? "check" : "file"} size={11} />
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      <pre dangerouslySetInnerHTML={{__html: highlightCode(text)}} />
    </div>
  );
}

function highlightCode(src) {
  let out = String(src)
    .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  const stash = [];
  const hold = (html) => { stash.push(html); return "\u0001S" + (stash.length - 1) + "\u0001"; };
  out = out.replace(/(\/\/[^\n]*)/g, (m) => hold('<span class="token-comm">' + m + '</span>'));
  out = out.replace(/('[^']*'|"[^"]*"|`[^`]*`)/g, (m) => hold('<span class="token-str">' + m + '</span>'));
  out = out.replace(
    /\b(async|await|return|const|let|var|if|else|new|throw|class|extends|export|import|from|interface|type|public|private|protected|readonly|enum|as|null|undefined|true|false|this|function)\b/g,
    (m) => hold('<span class="token-kw">' + m + '</span>')
  );
  out = out.replace(/\b([a-zA-Z_$][a-zA-Z0-9_$]*)(?=\()/g, (m) => hold('<span class="token-fn">' + m + '</span>'));
  out = out.replace(/\u0001S(\d+)\u0001/g, (_, i) => stash[+i]);
  return out;
}

function SearchResults({ results }) {
  return (
    <div className="search-results">
      {results.map((r, i) => (
        <div key={i} className="search-card">
          <div className="src">
            <span className="favicon">{r.domain[0].toUpperCase()}</span>
            <span style={{whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis'}}>{r.domain}</span>
            <span style={{marginLeft: 'auto', opacity: 0.7}}>{r.time}</span>
          </div>
          <div className="title">{r.title}</div>
          <div className="snip">{r.snippet}</div>
        </div>
      ))}
    </div>
  );
}

function ApprovalCard({ what, cmd, reason }) {
  const [state, setState] = React.useState("pending");
  if (state === "approved") {
    return (
      <div className="checkpoint">
        <div className="ico"><Icon name="check" size={11} strokeWidth={3} /></div>
        <span>Approved · running command</span>
      </div>
    );
  }
  if (state === "skipped") {
    return (
      <div className="checkpoint">
        <div className="ico" style={{color: 'var(--color-text-faint)'}}><Icon name="x" size={11} /></div>
        <span style={{color: 'var(--color-text-faint)'}}>Skipped</span>
      </div>
    );
  }
  return (
    <div className="approval-card">
      <div className="head">
        <Icon name="shield" size={12} />Approval required
      </div>
      <div className="what">{what}</div>
      <code className="cmd">$ {cmd}</code>
      <div className="reason">{reason}</div>
      <div className="actions">
        <button className="pill-btn accent" style={{height: 30, fontSize: 11}} onClick={() => setState("approved")}>Approve</button>
        <button className="pill-btn" style={{height: 30, fontSize: 11}} onClick={() => setState("skipped")}>Skip</button>
        <label className="always">
          <input type="checkbox" />
          Always allow pnpm test
        </label>
      </div>
    </div>
  );
}

function Checkpoint({ text }) {
  return (
    <div className="checkpoint">
      <div className="ico"><Icon name="check" size={11} strokeWidth={3} /></div>
      <span>{text}</span>
    </div>
  );
}

function Composer({ onSend, value, onChange, attachments, onRemoveAttachment, mode, onModeChange, branch, model, project, dirMode, onPickModel, onPickDir, onPickBranch }) {
  const inputRef = React.useRef(null);
  const submit = () => {
    if (!value.trim()) return;
    onSend(value.trim());
  };
  const onKey = (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      submit();
    }
  };
  React.useEffect(() => {
    if (inputRef.current) {
      inputRef.current.style.height = 'auto';
      inputRef.current.style.height = Math.min(inputRef.current.scrollHeight, 160) + 'px';
    }
  }, [value]);

  return (
    <React.Fragment>
      <div className="composer">
        {attachments.length > 0 && (
          <div className="composer-chips">
            {attachments.map((a, i) => (
              <span key={i} className="composer-chip">
                <Icon name={a.icon || "file"} size={11} />
                {a.label}
                <span className="x" onClick={() => onRemoveAttachment(i)}><Icon name="x" size={10} /></span>
              </span>
            ))}
          </div>
        )}
        <textarea
          ref={inputRef}
          className="composer-input"
          placeholder="Ask, plan, or paste a stack trace…  /  to run a command"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={onKey}
          rows={1}
        />
        <div className="composer-toolbar">
          <button className="composer-model" onClick={onPickModel} title="Switch model">
            <span className="cm-avatar">{(model || 'S').slice(0, 1)}</span>
            <span className="cm-name">{model}</span>
            <Icon name="more" size={10} />
          </button>
          <button className="composer-tool-btn" title="Attach file"><Icon name="paperclip" size={13} /></button>
          <button className={`composer-tool-btn ${mode==='agent' ? 'active' : ''}`} onClick={() => onModeChange('agent')} title="Agent mode">
            <Icon name="spark" size={12} />Agent
          </button>
          <button className={`composer-tool-btn ${mode==='ask' ? 'active' : ''}`} onClick={() => onModeChange('ask')} title="Ask mode">
            <Icon name="chat" size={12} />Ask
          </button>
          <button className={`composer-tool-btn ${mode==='plan' ? 'active' : ''}`} onClick={() => onModeChange('plan')} title="Plan mode">
            <Icon name="list" size={12} />Plan
          </button>
          <div className="spacer"></div>
          <div className="meta">
            <span className="accent">⌘K</span> commands · <span className="accent">⌘↵</span> send
          </div>
          <button className="send-btn" disabled={!value.trim()} onClick={submit} title="Send (⌘↵)">
            <Icon name="send-arrow" size={14} strokeWidth={2.5} />
          </button>
        </div>
      </div>
      <div className="composer-footer">
        <button className="cf-chip" onClick={onPickDir} title="Working directory">
          <Icon name="folder" size={11} />
          <span>{project}</span>
          <Icon name="more" size={10} />
        </button>
        <button className="cf-chip" onClick={onPickDir} title="Execution mode">
          <Icon name="shield" size={11} />
          <span>{dirMode}</span>
          <Icon name="more" size={10} />
        </button>
        <button className="cf-chip" onClick={onPickBranch} title="Git branch">
          <Icon name="branch" size={11} />
          <span>{branch}</span>
          <Icon name="more" size={10} />
        </button>
      </div>
    </React.Fragment>
  );
}

function Chat({
  branch, model, project, dirMode, running,
  messages, plan, toolCalls, selectedToolId, onSelectTool,
  composerValue, onComposerChange, onSend,
  attachments, onRemoveAttachment, mode, onModeChange,
  runStatus, onStop,
  tabs, activeTabId, onSelectTab, onCloseTab, onNewTab,
  expandedToolIds, onToggleExpand, onOpenInspector,
}) {
  return (
    <div className="panel chat">
      <ChatTopBar
        tabs={tabs}
        activeId={activeTabId}
        onSelect={onSelectTab}
        onClose={onCloseTab}
        onNew={onNewTab}
      />
      <div className="panel-scroll">
        <div className="msg-stream">
          {messages.map(m => (
            <MessageBlock
              key={m.id}
              msg={m}
              plan={plan}
              toolCalls={toolCalls}
              selectedToolId={selectedToolId}
              onSelectTool={onSelectTool}
              expandedIds={expandedToolIds}
              onToggleExpand={onToggleExpand}
              onOpenInspector={onOpenInspector}
            />
          ))}
        </div>
      </div>
      <div className="composer-wrap">
        <div className="composer-fade"></div>
        <div className="composer-inner">
          <StatusPill
            running={running}
            step={runStatus ? runStatus.step : 0}
            totalSteps={runStatus ? runStatus.totalSteps : 0}
            activity={runStatus ? runStatus.activity : null}
            tokens={runStatus ? runStatus.tokens : {used: '0', total: '0'}}
            ctxPct={runStatus ? runStatus.ctxPct : 0}
            cost={runStatus ? runStatus.cost : '0.00'}
            onStop={onStop}
          />
          <SlashSuggestions value={composerValue} onPick={(s) => onComposerChange(s)} />
          <Composer
            value={composerValue}
            onChange={onComposerChange}
            onSend={onSend}
            attachments={attachments}
            onRemoveAttachment={onRemoveAttachment}
            mode={mode}
            onModeChange={onModeChange}
            branch={branch}
            model={model}
            project={project}
            dirMode={dirMode}
          />
        </div>
      </div>
    </div>
  );
}

// Sticky progress bar that lives between the topbar and the messages.
// (Removed — info now consolidated into the StatusPill above messages.)

const SLASH_COMMANDS = [
  { cmd: "/explain", desc: "Explain a file, function, or selection" },
  { cmd: "/test", desc: "Generate or run tests for the current change" },
  { cmd: "/fix", desc: "Diagnose and fix the failing typecheck" },
  { cmd: "/diff", desc: "Show the working-tree diff inline" },
  { cmd: "/review", desc: "Review pending changes line-by-line" },
  { cmd: "/commit", desc: "Stage, commit, and push the current branch" },
  { cmd: "/search", desc: "Search the codebase for a symbol or pattern" },
  { cmd: "/plan", desc: "Restate or edit the current plan" },
];

function SlashSuggestions({ value, onPick }) {
  if (!value || !value.startsWith('/')) return null;
  const q = value.slice(1).toLowerCase();
  const filtered = SLASH_COMMANDS.filter(c => c.cmd.slice(1).startsWith(q)).slice(0, 5);
  if (filtered.length === 0) return null;
  return (
    <div className="slash-panel">
      <div className="slash-head">Commands</div>
      {filtered.map((c, i) => (
        <div key={c.cmd} className="slash-row" onClick={() => onPick(c.cmd + ' ')}>
          <code className="slash-cmd">{c.cmd}</code>
          <span className="slash-desc">{c.desc}</span>
          {i === 0 && <span className="slash-hint">↵</span>}
        </div>
      ))}
    </div>
  );
}

window.Chat = Chat;
