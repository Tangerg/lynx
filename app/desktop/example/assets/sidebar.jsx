// ============================================================
// Sonance Agent — Sidebar
// Two modes: rail (icon-strip) ⇄ expanded (full list)
// ============================================================

function Sidebar({ sessions, activeSessionId, onSelect, models, activeModelId, onModelClick, rail, onToggleRail, theme, accent, onToggleTheme, onAccentChange }) {
  const projects = window.PROJECTS;
  const active = models.find(m => m.id === activeModelId) || models[0];
  const recent = sessions.slice(0, 5);
  const [settingsOpen, setSettingsOpen] = React.useState(false);

  if (rail) {
    return (
      <div className="panel sidebar rail">
        <div className="rail-brand">
          <div className="brand-mark"><Icon name="spark" size={16} /></div>
        </div>
        <button className="rail-btn" title="Expand sidebar" onClick={onToggleRail}>
          <Icon name="panel-l" size={16} />
        </button>
        <button className="rail-btn primary" title="New session"><Icon name="plus" size={16} /></button>
        <button className="rail-btn" title="Search (⌘K)"><Icon name="search" size={16} /></button>
        <div className="rail-divider"></div>
        <div className="rail-sessions">
          {recent.map(s => (
            <button
              key={s.id}
              className={`rail-session ${s.id === activeSessionId ? 'active' : ''}`}
              title={s.title}
              onClick={() => onSelect(s.id)}
            >
              <span className="rail-session-glyph">{s.title.slice(0, 1).toUpperCase()}</span>
              {s.status === 'running' && <span className="rail-pulse"></span>}
              {s.status === 'waiting' && <span className="rail-pulse warn"></span>}
            </button>
          ))}
        </div>
        <div style={{flex: 1}}></div>
        <button className="rail-btn" title="Tools / MCP"><Icon name="tool" size={16} /></button>
        <button className="rail-btn" title="Settings"><Icon name="settings" size={16} /></button>
        <div className="rail-user" title="You · jdoe@longbridge-inc.com">J</div>
      </div>
    );
  }

  return (
    <div className="panel sidebar">
      <div className="brand">
        <div className="brand-mark"><Icon name="spark" size={16} /></div>
        <div>
          <div className="brand-name">Lynx</div>
        </div>
        <button className="brand-toggle" onClick={onToggleRail} title="Collapse to rail">
          <Icon name="panel-l" size={14} />
        </button>
      </div>

      <div className="side-search">
        <div className="side-search-icon"><Icon name="search" size={14} /></div>
        <input placeholder="Search · files · commands" />
        <span className="search-kbd">⌘K</span>
      </div>

      <div className="panel-scroll" style={{padding: '0 0 8px 0'}}>
        <div className="side-section-head">
          <span>Projects</span>
          <button className="add" title="Add project"><Icon name="plus" size={12} /></button>
        </div>
        <div className="side-list">
          {projects.map(p => (
            <div key={p.id} className={`session-row ${p.active ? 'active' : ''}`}>
              <div className="session-icon"><Icon name="folder" size={14} /></div>
              <div className="session-body">
                <div className="session-title">{p.name}</div>
                <div className="session-sub" style={{fontFamily: 'var(--font-mono)'}}>{p.branch}</div>
              </div>
            </div>
          ))}
        </div>

        <div className="side-section-head">
          <span>Sessions</span>
          <span className="count">{sessions.length}</span>
        </div>
        <div className="side-list">
          {sessions.map(s => (
            <div
              key={s.id}
              className={`session-row ${s.id === activeSessionId ? 'active' : ''}`}
              onClick={() => onSelect(s.id)}
            >
              <div className="session-icon"><Icon name="chat" size={14} /></div>
              <div className="session-body">
                <div className="session-title">{s.title}</div>
                <div className="session-sub">
                  <span className={`status-dot ${s.status}`}></span>
                  <span>{s.status === 'running' ? 'Running' : s.status === 'waiting' ? 'Needs input' : s.model}</span>
                </div>
              </div>
              <div className="session-time">{s.time}</div>
            </div>
          ))}
        </div>
      </div>

      <div className="side-footer">
        <div className="user-card">
          <div className="user-avatar">J</div>
          <div className="user-body">
            <div className="user-name">Jamie Doe</div>
            <div className="user-sub">jdoe@longbridge-inc.com</div>
          </div>
          <div className="user-settings-wrap">
            <button
              className={`user-action ${settingsOpen ? 'open' : ''}`}
              onClick={() => setSettingsOpen(o => !o)}
              title="Preferences"
            ><Icon name="settings" size={14} /></button>
            {settingsOpen && (
              <div className="settings-popover" onMouseLeave={() => setSettingsOpen(false)}>
                <div className="sp-section">Appearance</div>
                <div className="sp-row">
                  <span className="sp-label">Theme</span>
                  <div className="sp-segmented">
                    <button
                      className={`sp-seg ${theme !== 'light' ? 'active' : ''}`}
                      onClick={() => onToggleTheme && (theme === 'light' ? onToggleTheme() : null)}
                    >
                      <Icon name="moon" size={11} />Dark
                    </button>
                    <button
                      className={`sp-seg ${theme === 'light' ? 'active' : ''}`}
                      onClick={() => onToggleTheme && (theme !== 'light' ? onToggleTheme() : null)}
                    >
                      <Icon name="sun" size={11} />Light
                    </button>
                  </div>
                </div>
                <div className="sp-row">
                  <span className="sp-label">Accent</span>
                  <div className="accent-swatches">
                    {[
                      { name: 'green',  color: '#1ed760' },
                      { name: 'blue',   color: '#82cfff' },
                      { name: 'pink',   color: '#e07acc' },
                      { name: 'orange', color: '#ffa42b' },
                    ].map(s => (
                      <button
                        key={s.name}
                        className={`accent-swatch ${accent === s.color ? 'active' : ''}`}
                        style={{ background: s.color }}
                        onClick={() => onAccentChange(s.color)}
                        title={`Accent: ${s.name}`}
                      ></button>
                    ))}
                  </div>
                </div>
              </div>
            )}
          </div>
          <button className="user-action" title="Account menu"><Icon name="more" size={14} /></button>
        </div>
      </div>
    </div>
  );
}

window.Sidebar = Sidebar;
