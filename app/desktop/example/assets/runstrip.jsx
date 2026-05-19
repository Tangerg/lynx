// ============================================================
// Sonance Agent — Run status strip
// Slim informational strip above the composer. No playback metaphor.
// ============================================================

function RunStrip({ running, step, totalSteps, activity, tokens, ctxPct, cost, onStop }) {
  return (
    <div className="run-strip">
      <div className={`step ${running ? '' : 'paused'}`}>
        <span className="dot"></span>
        <span>{running ? 'Running' : 'Paused'}</span>
      </div>
      <div className="activity">{activity}</div>
      <div className="stat" title="Context window usage">
        <span className="ctx-bar"><div style={{width: ctxPct + '%'}}></div></span>
        <span><span className="v">{tokens.used}</span> / {tokens.total}</span>
      </div>
      <div className="stat" title="Session cost">
        <span style={{color: 'var(--color-warning)'}}>$</span>
        <span className="v">{cost}</span>
      </div>
      {running && (
        <button className="stop-link" onClick={onStop} title="Stop the agent (⌘.)">
          <Icon name="stop" size={10} />Stop
        </button>
      )}
    </div>
  );
}

window.RunStrip = RunStrip;
