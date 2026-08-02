const {Icon, Kbd, ListBox} = window.DesignSystem_df98fe;

/* ⌘K palette — the docs search overlay. */
function SearchPalette({isOpen, onClose, items, onPick}) {
  const [q, setQ] = React.useState('');
  if (!isOpen) return null;
  const matches = items.filter(i => i.toLowerCase().includes(q.toLowerCase())).slice(0, 6);
  return (
    <div className="backdrop" style={{alignItems:'flex-start',paddingTop:'12vh'}} onClick={onClose}>
      <div className="modal" style={{maxWidth:520}} onClick={e => e.stopPropagation()}>
        <div style={{display:'flex',alignItems:'center',gap:10,padding:'14px 16px',borderBottom:'1px solid var(--separator)'}}>
          <Icon name="search" size={18} />
          <input autoFocus value={q} onChange={e => setQ(e.target.value)} placeholder="Search components, guides…"
            style={{flex:1,border:0,outline:'none',background:'transparent',font:'400 15px var(--font-sans)',color:'var(--foreground)'}} />
          <Kbd>Esc</Kbd>
        </div>
        <div style={{padding:8}}>
          <ListBox style={{boxShadow:'none',background:'transparent',padding:0}}>
            {matches.length === 0
              ? <div style={{padding:'20px 12px',textAlign:'center',color:'var(--muted)',fontSize:13}}>No results for "{q}"</div>
              : matches.map(m => (
                  <ListBox.Item key={m} startContent={<Icon name="layers" size={15} />} onClick={() => { onPick(m); onClose(); }}>
                    {m}
                    <span className="dropdown__shortcut">Components</span>
                  </ListBox.Item>
                ))}
          </ListBox>
        </div>
      </div>
    </div>
  );
}

Object.assign(window, {SearchPalette});
