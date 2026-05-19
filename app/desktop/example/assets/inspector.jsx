// ============================================================
// Sonance Agent — Right Inspector Panel (slide-out sheet)
// Tabs: Diff · Terminal · Files · Plan · Tools
// ============================================================

function highlightTS(code) {
  // Tokenize via placeholders so later passes don't match span attribute text.
  let out = String(code)
    .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  const stash = [];
  const hold = (html) => { stash.push(html); return "\u0001STASH" + (stash.length - 1) + "\u0001"; };
  out = out.replace(/(\/\/[^\n]*)/g, (m) => hold('<span class="token-comm">' + m + '</span>'));
  out = out.replace(/('[^']*'|"[^"]*")/g, (m) => hold('<span class="token-str">' + m + '</span>'));
  out = out.replace(
    /\b(async|await|return|const|let|var|if|else|new|throw|class|extends|export|import|from|interface|type|public|private|protected|readonly|enum|as|null|undefined|true|false|this)\b/g,
    (m) => hold('<span class="token-kw">' + m + '</span>')
  );
  out = out.replace(/\b([a-zA-Z_$][a-zA-Z0-9_$]*)(?=\()/g, (m) => hold('<span class="token-fn">' + m + '</span>'));
  out = out.replace(/\u0001STASH(\d+)\u0001/g, (_, i) => stash[+i]);
  return out;
}

function DiffView({ diff }) {
  return (
    <div className="diff-view">
      {diff.map((row, i) => {
        if (row.type === "hunk") {
          return <div key={i} className="diff-hunk-head">{row.text}</div>;
        }
        const cls = row.type === "add" ? "add" : row.type === "del" ? "del" : "ctx";
        const sign = row.type === "add" ? "+" : row.type === "del" ? "−" : " ";
        const lnum = row.type === "del" ? row.l : row.type === "add" ? row.r : row.r;
        return (
          <div key={i} className={`diff-line ${cls}`}>
            <span className="ln">{lnum}</span>
            <span className="sign">{sign}</span>
            <span className="code" dangerouslySetInnerHTML={{__html: highlightTS(row.code)}} />
          </div>
        );
      })}
    </div>
  );
}

function Terminal({ lines, running }) {
  return (
    <div className="term">
      {lines.map((l, i) => (
        <span key={i} className={l.kind}>{l.text}</span>
      ))}
      {running && (
        <span style={{display: 'inline-flex', alignItems: 'center', gap: 8, marginTop: 8, color: 'var(--color-info)'}}>
          <span style={{width: 8, height: 8, background: 'currentColor', borderRadius: '50%', animation: 'pulse 1.2s ease-in-out infinite'}}></span>
          tsc watching for changes…
        </span>
      )}
    </div>
  );
}

function FilesChanged({ files, activePath, onSelect }) {
  return (
    <div className="tree">
      <div style={{
        fontSize: 10.5, fontWeight: 700, letterSpacing: '0.14em', textTransform: 'uppercase',
        color: 'var(--color-text-faint)', padding: '8px 10px 8px 10px',
        display: 'flex', alignItems: 'center', gap: 8
      }}>
        <span>{files.length} files changed</span>
        <span style={{marginLeft: 'auto', color: 'var(--color-accent)'}}>+{files.reduce((s,f)=>s+f.added,0)}</span>
        <span style={{color: 'var(--color-negative)'}}>−{files.reduce((s,f)=>s+f.removed,0)}</span>
      </div>
      {files.map(f => (
        <div
          key={f.path}
          className={`tree-row ${f.path === activePath ? 'active' : ''}`}
          onClick={() => onSelect && onSelect(f.path)}
        >
          <Icon name="file" size={12} />
          <span style={{
            fontSize: 9, fontWeight: 700, letterSpacing: '0.04em',
            color: f.change === 'add' ? 'var(--color-accent)' : f.change === 'del' ? 'var(--color-negative)' : 'var(--color-warning)',
            textTransform: 'uppercase'
          }}>{f.change === 'add' ? 'A' : f.change === 'del' ? 'D' : 'M'}</span>
          <span className="name">{f.path}</span>
          <span style={{display: 'flex', gap: 6, fontSize: 10}}>
            <span style={{color: 'var(--color-accent)'}}>+{f.added}</span>
            <span style={{color: 'var(--color-negative)'}}>−{f.removed}</span>
          </span>
        </div>
      ))}
    </div>
  );
}

function PlanInspector({ plan }) {
  return (
    <div style={{padding: '14px 18px'}}>
      <div style={{fontSize: 10.5, fontWeight: 700, letterSpacing: '0.14em', textTransform: 'uppercase', color: 'var(--color-text-faint)', marginBottom: 12}}>
        Task plan
      </div>
      {plan.map(p => (
        <div key={p.id} className={`plan-item ${p.status}`} style={{padding: '8px 0'}}>
          <div className="check">
            {p.status === 'done' && <Icon name="check" size={12} strokeWidth={3} />}
          </div>
          <div>{p.text}</div>
        </div>
      ))}
      <div style={{
        marginTop: 16, padding: '12px 14px',
        background: 'var(--color-surface)', borderRadius: 8,
        fontSize: 12, color: 'var(--color-text-muted)', lineHeight: 1.5
      }}>
        <div style={{display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6}}>
          <Icon name="shield" size={13} style={{color: 'var(--color-warning)'}} />
          <span style={{fontWeight: 700, color: 'var(--color-text)', fontSize: 11.5, letterSpacing: '0.04em', textTransform: 'uppercase'}}>Approval required</span>
        </div>
        Agent will run <code style={{
          fontFamily: 'var(--font-mono)', background: 'var(--color-surface-2)',
          padding: '1px 5px', borderRadius: 3, color: 'var(--color-text)'
        }}>pnpm test --filter=auth</code> after typecheck passes.
        <div style={{display: 'flex', gap: 6, marginTop: 10}}>
          <button className="pill-btn accent" style={{height: 26, fontSize: 10.5, padding: '0 12px'}}>Approve</button>
          <button className="pill-btn" style={{height: 26, fontSize: 10.5, padding: '0 12px'}}>Skip</button>
        </div>
      </div>
    </div>
  );
}

function Inspector({ open, tab, onTab, onClose, selectedTool, files, activeFile, onSelectFile, plan }) {
  const counts = {
    files: files.length,
    plan: plan.filter(p => p.status !== 'done').length,
    tools: window.MCP_SERVERS.filter(s => s.status === 'active').length,
  };

  const railBtns = [
    { id: 'diff', icon: 'diff', label: 'Diff' },
    { id: 'terminal', icon: 'terminal', label: 'Terminal' },
    { id: 'files', icon: 'filetext', label: 'Files', badge: counts.files },
    { id: 'plan', icon: 'list', label: 'Plan', badge: counts.plan },
    { id: 'tools', icon: 'tool', label: 'Tools', badge: counts.tools },
  ];

  // Click a rail icon:
  //  - if it's already the active tab AND inspector content is open → collapse
  //  - otherwise → open + switch to that tab
  const handleRailClick = (b) => {
    if (open && tab === b.id) onClose();
    else onTab(b.id);
  };

  return (
    <div className={`panel inspector inspector-sheet ${open ? 'open' : 'closed'}`}>
      <div className="insp-rail">
        {railBtns.map(b => (
          <button
            key={b.id}
            className={`insp-rail-btn ${open && tab === b.id ? 'active' : ''}`}
            onClick={() => handleRailClick(b)}
            title={b.label}
          >
            <Icon name={b.icon} size={15} />
            {b.badge > 0 && <span className="rail-badge">{b.badge}</span>}
          </button>
        ))}
        <div className="insp-rail-spacer"></div>
        <div className="insp-rail-bottom">
          {open && (
            <button className="insp-rail-btn" onClick={onClose} title="Collapse panel (⌘\)">
              <Icon name="panel" size={15} />
            </button>
          )}
        </div>
      </div>

      <div className="insp-content">

      {tab === 'diff' && (
        <React.Fragment>
          <div className="insp-head">
            <div className="ficon"><Icon name="file" size={14} /></div>
            <div style={{minWidth: 0}}>
              <div className="ftitle">{activeFile || "src/api/auth.ts"}</div>
              <div className="fsub">
                <span style={{color: 'var(--color-accent)'}}>+47</span>
                <span style={{margin: '0 4px'}}>·</span>
                <span style={{color: 'var(--color-negative)'}}>−31</span>
                <span style={{margin: '0 8px', color: 'var(--color-text-faint)'}}>·</span>
                <span>247 lines · TypeScript</span>
              </div>
            </div>
            <div style={{display: 'flex', gap: 4}}>
              <button className="icon-btn" title="Revert"><Icon name="loop" size={14} /></button>
              <button className="icon-btn" title="Accept"><Icon name="check" size={14} /></button>
            </div>
          </div>
          <div className="panel-scroll"><DiffView diff={window.DIFF} /></div>
        </React.Fragment>
      )}

      {tab === 'terminal' && (
        <React.Fragment>
          <div className="insp-head">
            <div className="ficon"><Icon name="terminal" size={14} /></div>
            <div style={{minWidth: 0}}>
              <div className="ftitle">pnpm typecheck</div>
              <div className="fsub">
                <span style={{color: 'var(--color-info)'}}>● Running</span>
                <span style={{margin: '0 8px', color: 'var(--color-text-faint)'}}>·</span>
                <span>1 error · 1 warning</span>
                <span style={{margin: '0 8px', color: 'var(--color-text-faint)'}}>·</span>
                <span>~/code/fern-api</span>
              </div>
            </div>
            <div style={{display: 'flex', gap: 4}}>
              <button className="icon-btn" title="Re-run"><Icon name="loop" size={14} /></button>
              <button className="icon-btn" title="Stop"><Icon name="stop" size={12} /></button>
            </div>
          </div>
          <div className="panel-scroll"><Terminal lines={window.TERM_LINES} running={true} /></div>
        </React.Fragment>
      )}

      {tab === 'files' && (
        <React.Fragment>
          <div className="insp-head">
            <div className="ficon"><Icon name="filetext" size={14} /></div>
            <div style={{minWidth: 0}}>
              <div className="ftitle" style={{fontFamily: 'var(--font-ui)', fontSize: 13, fontWeight: 700}}>Working tree</div>
              <div className="fsub">{files.length} files · uncommitted</div>
            </div>
            <div style={{display: 'flex', gap: 4}}>
              <button className="icon-btn" title="Stage all"><Icon name="check" size={14} /></button>
              <button className="icon-btn" title="More"><Icon name="more" size={14} /></button>
            </div>
          </div>
          <div className="panel-scroll"><FilesChanged files={files} activePath={activeFile} onSelect={(p) => { onSelectFile(p); onTab('diff'); }} /></div>
        </React.Fragment>
      )}

      {tab === 'plan' && (
        <React.Fragment>
          <div className="insp-head">
            <div className="ficon"><Icon name="list" size={14} /></div>
            <div style={{minWidth: 0}}>
              <div className="ftitle" style={{fontFamily: 'var(--font-ui)', fontSize: 13, fontWeight: 700}}>Refactor auth.ts → Result</div>
              <div className="fsub">{plan.filter(p=>p.status==='done').length} of {plan.length} complete · est. 2 min remaining</div>
            </div>
            <div style={{display: 'flex', gap: 4}}>
              <button className="icon-btn" title="Edit plan"><Icon name="edit" size={14} /></button>
            </div>
          </div>
          <div className="panel-scroll"><PlanInspector plan={plan} /></div>
        </React.Fragment>
      )}

      {tab === 'tools' && (
        <React.Fragment>
          <div className="insp-head">
            <div className="ficon"><Icon name="tool" size={14} /></div>
            <div style={{minWidth: 0}}>
              <div className="ftitle" style={{fontFamily: 'var(--font-ui)', fontSize: 13, fontWeight: 700}}>Connected MCP servers</div>
              <div className="fsub">{counts.tools} active · {window.MCP_SERVERS.length} configured</div>
            </div>
            <div style={{display: 'flex', gap: 4}}>
              <button className="icon-btn" title="Add server"><Icon name="plus" size={14} /></button>
            </div>
          </div>
          <div className="panel-scroll" style={{padding: '4px 0'}}>
            {window.MCP_SERVERS.map(s => (
              <div key={s.id} className={`mcp-row ${s.status}`}>
                <div className="mcp-icon"><Icon name={s.icon} size={15} /></div>
                <div style={{minWidth: 0}}>
                  <div className="mcp-name">{s.name}</div>
                  <div className="mcp-desc">{s.desc}</div>
                </div>
                <div className="mcp-tools">{s.tools} tools</div>
                <div className={`mcp-status ${s.status}`}>
                  {s.status === 'active' ? 'On' : s.status === 'idle' ? 'Idle' : 'Error'}
                </div>
              </div>
            ))}
            <div style={{padding: '14px 16px 18px 16px', color: 'var(--color-text-faint)', fontSize: 11, lineHeight: 1.5}}>
              Servers expose tools the agent can call. Edit <code style={{fontFamily: 'var(--font-mono)', background: 'var(--color-surface-2)', padding: '1px 5px', borderRadius: 3, color: 'var(--color-text)'}}>~/.sonance/mcp.json</code> to add or remove.
            </div>
          </div>
        </React.Fragment>
      )}

      </div>
    </div>
  );
}

window.Inspector = Inspector;
