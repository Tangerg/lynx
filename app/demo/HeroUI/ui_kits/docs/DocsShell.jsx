const {Button, Icon, Kbd, Chip, Link, Separator, Avatar} = window.DesignSystem_df98fe;

/* Top bar of heroui.com/docs — product mark, primary nav, version, search, theme, stars. */
function DocsTopBar({theme, onTheme, onSearch, section, onSection}) {
  return (
    <header style={{position:'sticky',top:0,zIndex:30,background:'color-mix(in oklab, var(--background) 82%, transparent)',backdropFilter:'blur(12px)',borderBottom:'1px solid var(--separator)'}}>
      <div style={{display:'flex',alignItems:'center',gap:20,height:56,padding:'0 20px'}}>
        <span style={{fontWeight:700,fontSize:17,letterSpacing:'-0.04em'}}>HeroUI</span>
        <Chip size="sm" variant="outline">v3.2.2</Chip>
        <nav style={{display:'flex',gap:4,marginInlineStart:8}}>
          {['Getting Started','Components','Releases','Migration'].map(n => (
            <button key={n} type="button" onClick={() => onSection(n)}
              style={{border:0,background:'transparent',font:'500 14px var(--font-sans)',padding:'6px 10px',borderRadius:'var(--radius-lg)',cursor:'pointer',whiteSpace:'nowrap',
                color: section===n ? 'var(--foreground)' : 'var(--muted)'}}>{n}</button>
          ))}
        </nav>
        <div style={{marginInlineStart:'auto',display:'flex',alignItems:'center',gap:8}}>
          <button type="button" onClick={onSearch} className="input-group"
            style={{width:210,height:36,cursor:'pointer',color:'var(--field-placeholder)'}}>
            <span className="input-group__addon"><Icon name="search" /></span>
            <span style={{flex:1,textAlign:'start',fontSize:13}}>Search</span>
            <Kbd keys={['cmd','k']} />
          </button>
          <div className="toggle-button-group">
            <button type="button" className="button toggle-button button--sm button--icon-only"
              data-selected={theme==='light'?'true':undefined} onClick={() => onTheme('light')} aria-label="Light"><Icon name="sun" /></button>
            <button type="button" className="button toggle-button button--sm button--icon-only"
              data-selected={theme==='dark'?'true':undefined} onClick={() => onTheme('dark')} aria-label="Dark"><Icon name="moon" /></button>
          </div>
          <Button variant="ghost" size="sm" startIcon={<Icon name="code" />}>27.7k</Button>
        </div>
      </div>
    </header>
  );
}

/* Left rail: grouped component index. */
function DocsSidebar({groups, current, onSelect, query, onQuery}) {
  return (
    <aside style={{width:230,flexShrink:0,paddingBlock:20,paddingInlineEnd:12,borderInlineEnd:'1px solid var(--separator)'}}>
      <div className="search-field" style={{marginBottom:14}}>
        <span className="search-field__icon"><Icon name="search" size={14} /></span>
        <input className="input input--sm" placeholder="Filter" value={query} onChange={e => onQuery(e.target.value)} />
      </div>
      <div style={{display:'flex',flexDirection:'column',gap:16,maxHeight:520,overflow:'auto'}} className="scrollbar">
        {groups.map(g => {
          const items = g.items.filter(i => i.toLowerCase().includes(query.toLowerCase()));
          if (!items.length) return null;
          return (
            <div key={g.label} style={{display:'flex',flexDirection:'column',gap:2}}>
              <span style={{fontSize:11,fontWeight:600,color:'var(--muted)',padding:'0 10px 4px',textTransform:'uppercase',letterSpacing:'.04em'}}>{g.label}</span>
              {items.map(i => (
                <button key={i} type="button" onClick={() => onSelect(i)}
                  style={{border:0,textAlign:'start',cursor:'pointer',padding:'6px 10px',borderRadius:'var(--radius-lg)',font:'400 13.5px var(--font-sans)',
                    background: current===i ? 'var(--default)' : 'transparent',
                    color: current===i ? 'var(--foreground)' : 'var(--muted)',
                    fontWeight: current===i ? 500 : 400}}>{i}</button>
              ))}
            </div>
          );
        })}
      </div>
    </aside>
  );
}

/* Right rail: on-this-page. */
function DocsToc({items, active}) {
  return (
    <aside style={{width:190,flexShrink:0,paddingBlock:24,paddingInlineStart:20}}>
      <span style={{fontSize:11,fontWeight:600,color:'var(--muted)',textTransform:'uppercase',letterSpacing:'.04em'}}>On this page</span>
      <div style={{display:'flex',flexDirection:'column',gap:6,marginTop:10}}>
        {items.map(i => (
          <a key={i} href="#" onClick={e => e.preventDefault()}
            style={{fontSize:13,textDecoration:'none',color: i===active ? 'var(--foreground)' : 'var(--muted)', fontWeight: i===active ? 500 : 400}}>{i}</a>
        ))}
      </div>
    </aside>
  );
}

Object.assign(window, {DocsTopBar, DocsSidebar, DocsToc});
