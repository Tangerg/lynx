// ============================================================
// Sonance Agent — App root
// New layout: rail sidebar + top tab bar + slide-out inspector
// ============================================================

const TWEAK_DEFAULTS = /*EDITMODE-BEGIN*/{
  "sidebarRail": false,
  "inspectorOpen": false,
  "accent": "#1ed760",
  "theme": "dark"
}/*EDITMODE-END*/;

function App() {
  const [t, setTweak] = useTweaks(TWEAK_DEFAULTS);

  React.useEffect(() => {
    const isLight = (t.theme || 'dark') === 'light';
    const lightMap = {
      '#1ed760': '#169c46',
      '#82cfff': '#2563eb',
      '#e07acc': '#a823a3',
      '#ffa42b': '#c2410c',
    };
    const accent = isLight ? (lightMap[t.accent] || t.accent) : t.accent;
    document.documentElement.style.setProperty('--color-accent', accent);
  }, [t.accent, t.theme]);

  React.useEffect(() => {
    document.documentElement.classList.remove('theme-light', 'theme-dark');
    document.documentElement.classList.add('theme-' + (t.theme || 'dark'));
  }, [t.theme]);

  const [sessions, setSessions] = React.useState(window.SESSIONS);
  const [activeSession, setActiveSession] = React.useState("s1");
  const activeS = sessions.find(s => s.id === activeSession) || sessions[0];

  // Open tabs at the top of the chat panel — defaults to a few sessions open.
  const [tabIds, setTabIds] = React.useState(["s1", "s2", "s3"]);
  const openTabs = tabIds.map(id => sessions.find(s => s.id === id)).filter(Boolean);
  const ensureTabOpen = (id) => {
    setTabIds(prev => prev.includes(id) ? prev : [...prev, id]);
  };
  const closeTab = (id) => {
    setTabIds(prev => {
      const next = prev.filter(x => x !== id);
      if (id === activeSession && next.length > 0) setActiveSession(next[0]);
      return next;
    });
  };
  const newTab = () => {
    const candidate = sessions.find(s => !tabIds.includes(s.id));
    if (candidate) {
      setTabIds(prev => [...prev, candidate.id]);
      setActiveSession(candidate.id);
    }
  };
  const selectTab = (id) => {
    setActiveSession(id);
    ensureTabOpen(id);
  };

  // Sidebar rail mode + inspector sheet
  const [sidebarRail, setSidebarRail] = React.useState(t.sidebarRail);
  const [inspectorOpen, setInspectorOpen] = React.useState(t.inspectorOpen);
  React.useEffect(() => { setSidebarRail(t.sidebarRail); }, [t.sidebarRail]);
  React.useEffect(() => { setInspectorOpen(t.inspectorOpen); }, [t.inspectorOpen]);

  const toggleSidebar = () => {
    const next = !sidebarRail;
    setSidebarRail(next);
    setTweak('sidebarRail', next);
  };
  const toggleInspector = () => {
    const next = !inspectorOpen;
    setInspectorOpen(next);
    setTweak('inspectorOpen', next);
  };

  const [messages, setMessages] = React.useState(window.INITIAL_MESSAGES);
  const [toolCalls, setToolCalls] = React.useState(window.TOOL_CALLS);
  const [plan, setPlan] = React.useState(window.PLAN);
  const [selectedToolId, setSelectedToolId] = React.useState("t4");

  // Tools that are expanded inline in the chat
  const [expandedToolIds, setExpandedToolIds] = React.useState(new Set(["t4"]));
  const toggleExpand = (id) => {
    setExpandedToolIds(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  };

  const [composer, setComposer] = React.useState("");
  const [composerMode, setComposerMode] = React.useState("agent");
  const [attachments, setAttachments] = React.useState([
    { label: "src/api/auth.ts", icon: "file" },
    { label: "TYPES.md", icon: "filetext" },
  ]);
  const removeAttachment = (i) => setAttachments(a => a.filter((_, idx) => idx !== i));

  // Inspector state
  const [tab, setTab] = React.useState("diff");
  const [activeFile, setActiveFile] = React.useState("src/api/auth.ts");

  const openToolInInspector = (toolId) => {
    setSelectedToolId(toolId);
    const tool = toolCalls.find(x => x.id === toolId);
    if (!tool) return;
    if (tool.fn === "bash") setTab("terminal");
    else if (tool.fn === "edit_file" || tool.fn === "write_file" || tool.fn === "read_file") {
      setTab("diff");
      const m = String(tool.args).match(/^([^\s(]+)/);
      if (m) setActiveFile(m[1]);
    } else setTab("diff");
    if (!inspectorOpen) {
      setInspectorOpen(true);
      setTweak('inspectorOpen', true);
    }
  };

  // Selecting a tool just selects it; user can click again to expand inline,
  // or use the "Open in inspector" button inside the inline preview.
  const selectTool = (toolId) => setSelectedToolId(toolId);

  // Running state
  const [running, setRunning] = React.useState(true);
  const [elapsed, setElapsed] = React.useState(34);
  React.useEffect(() => {
    if (!running) return;
    const id = setInterval(() => setElapsed(e => e + 1), 1000);
    return () => clearInterval(id);
  }, [running]);
  const fmt = (s) => `${Math.floor(s/60)}:${String(s%60).padStart(2, "0")}`;

  const ACTIVITY = [
    "pnpm typecheck · src/api/billing.ts",
    "pnpm typecheck · src/components/LoginForm.tsx",
    "pnpm typecheck · waiting for tsc watcher",
    "Reading src/api/billing.ts:142",
    "Scanning result of pnpm test --filter=auth",
  ];
  const [activityIdx, setActivityIdx] = React.useState(0);
  React.useEffect(() => {
    if (!running) return;
    const id = setInterval(() => setActivityIdx(i => (i + 1) % ACTIVITY.length), 2400);
    return () => clearInterval(id);
  }, [running]);

  const handleSend = (text) => {
    const now = new Date();
    const time = `${(now.getHours()%12||12)}:${String(now.getMinutes()).padStart(2,"0")} ${now.getHours()>=12?'PM':'AM'}`;
    const userMsg = {
      id: "u" + Date.now(),
      role: "user", who: "You", time,
      content: [{ type: "text", text }],
    };
    const agentMsg = {
      id: "a" + Date.now(),
      role: "agent", who: "Sonnet 4.5", time,
      content: [{ type: "stream", text: "Thinking" }],
    };
    setMessages(m => [...m, userMsg, agentMsg]);
    setComposer("");
    setRunning(true);

    const replies = [
      "Got it — let me trace where this lives.",
      " I'll search the tree, read the relevant files, and propose a diff before touching anything irreversible.",
    ];
    const fullText = replies.join("");
    let i = 0;
    const tick = setInterval(() => {
      i = Math.min(fullText.length, i + 3);
      setMessages(prev => {
        const next = [...prev];
        const last = next[next.length - 1];
        if (last && last.role === 'agent') {
          last.content = [{ type: "stream", text: fullText.slice(0, i) }];
        }
        return next;
      });
      if (i >= fullText.length) {
        clearInterval(tick);
        setMessages(prev => {
          const next = [...prev];
          const last = next[next.length - 1];
          if (last && last.role === 'agent') {
            last.content = [{ type: "text", text: fullText }];
          }
          return next;
        });
      }
    }, 28);
  };

  React.useEffect(() => {
    const tick = () => {
      const el = document.querySelector('.chat .panel-scroll');
      if (el) el.scrollTop = el.scrollHeight;
    };
    tick();
    setTimeout(tick, 50);
    setTimeout(tick, 250);
  }, [messages]);

  const runStatus = {
    running,
    step: 5,
    totalSteps: 7,
    activity: ACTIVITY[activityIdx],
    tokens: { used: "47.2k", total: "200k" },
    ctxPct: 24,
    cost: "0.34",
  };

  return (
    <div className={`app ${sidebarRail ? 'rail' : ''} ${inspectorOpen ? 'insp-open' : ''}`}>
      <div className="app-main">
        <Sidebar
          sessions={sessions}
          activeSessionId={activeSession}
          onSelect={selectTab}
          models={window.MODELS}
          activeModelId="sonnet45"
          onModelClick={() => {}}
          rail={sidebarRail}
          onToggleRail={toggleSidebar}
          theme={t.theme || 'dark'}
          accent={t.accent}
          onToggleTheme={() => setTweak('theme', (t.theme || 'dark') === 'light' ? 'dark' : 'light')}
          onAccentChange={(c) => setTweak('accent', c)}
        />
        <Chat
          branch="feat/result-type"
          model={activeS.model}
          project="fern-api"
          dirMode="Workspace · Auto"
          running={running && activeS.status === 'running'}
          messages={messages}
          plan={plan}
          toolCalls={toolCalls}
          selectedToolId={selectedToolId}
          onSelectTool={selectTool}
          composerValue={composer}
          onComposerChange={setComposer}
          onSend={handleSend}
          attachments={attachments}
          onRemoveAttachment={removeAttachment}
          mode={composerMode}
          onModeChange={setComposerMode}
          runStatus={runStatus}
          onStop={() => setRunning(false)}
          tabs={openTabs}
          activeTabId={activeSession}
          onSelectTab={selectTab}
          onCloseTab={closeTab}
          onNewTab={newTab}
          expandedToolIds={expandedToolIds}
          onToggleExpand={toggleExpand}
          onOpenInspector={openToolInInspector}
        />
        <Inspector
          open={inspectorOpen}
          tab={tab}
          onTab={(t) => {
            setTab(t);
            if (!inspectorOpen) {
              setInspectorOpen(true);
              setTweak('inspectorOpen', true);
            }
          }}
          onClose={toggleInspector}
          selectedTool={toolCalls.find(x => x.id === selectedToolId)}
          files={window.FILES_CHANGED}
          activeFile={activeFile}
          onSelectFile={setActiveFile}
          plan={plan}
        />
      </div>

      <TweaksPanel title="Tweaks">
        <TweakSection label="Theme" />
        <TweakRadio
          label="Mode"
          value={t.theme || 'dark'}
          options={['dark', 'light']}
          onChange={(v) => setTweak('theme', v)}
        />
        <TweakColor
          label="Accent"
          value={t.accent}
          options={["#1ed760", "#82cfff", "#e07acc", "#ffa42b"]}
          onChange={(v) => setTweak('accent', v)}
        />
        <TweakSection label="Layout" />
        <TweakToggle
          label="Rail sidebar"
          value={sidebarRail}
          onChange={(v) => { setSidebarRail(v); setTweak('sidebarRail', v); }}
        />
        <TweakToggle
          label="Inspector open"
          value={inspectorOpen}
          onChange={(v) => { setInspectorOpen(v); setTweak('inspectorOpen', v); }}
        />
      </TweaksPanel>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(<App />);
