const {Button, Icon, Chip, Tabs, Link, Separator, Typography, Card, TextField, Checkbox, Switch, Chip: HChip} = window.DesignSystem_df98fe;

/* The body of a component doc page: title, meta links, live preview, CSS class list, props table. */
function ComponentPage({name, description, preview, classes, props}) {
  return (
    <article style={{flex:1,minWidth:0,padding:'24px 28px',display:'flex',flexDirection:'column',gap:22}}>
      <div style={{display:'flex',flexDirection:'column',gap:10}}>
        <div style={{display:'flex',alignItems:'center',gap:10}}>
          <h1 className="typography typography--h2">{name}</h1>
          <Button variant="ghost" size="sm" startIcon={<Icon name="copy" />}>Copy Markdown</Button>
        </div>
        <p className="typography typography--body" style={{color:'var(--muted)',maxWidth:'68ch'}}>{description}</p>
        <div style={{display:'flex',gap:8}}>
          <Chip size="sm" variant="outline">Storybook</Chip>
          <Chip size="sm" variant="outline">Source</Chip>
          <Chip size="sm" variant="outline">Styles source</Chip>
        </div>
      </div>

      <section style={{display:'flex',flexDirection:'column',gap:10}}>
        <h2 className="typography typography--h4">Usage</h2>
        <Tabs variant="underline" defaultSelectedKey="preview">
          <Tabs.ListContainer>
            <Tabs.Tab id="preview">Preview</Tabs.Tab>
            <Tabs.Tab id="code">Code</Tabs.Tab>
          </Tabs.ListContainer>
          <Tabs.Panel id="preview">
            <div style={{border:'1px solid var(--separator)',borderRadius:'var(--radius-2xl)',padding:'28px 24px',display:'flex',alignItems:'center',justifyContent:'center',gap:12,flexWrap:'wrap',minHeight:120}}>
              {preview}
            </div>
          </Tabs.Panel>
          <Tabs.Panel id="code">
            <pre style={{margin:0,background:'var(--surface-secondary)',borderRadius:'var(--radius-2xl)',padding:18,fontFamily:'var(--font-mono)',fontSize:13,lineHeight:1.7,overflow:'auto'}}>
{`import {${name}} from "@heroui/react";

export default function Demo() {
  return <${name} />;
}`}
            </pre>
          </Tabs.Panel>
        </Tabs>
      </section>

      <section style={{display:'flex',flexDirection:'column',gap:10}}>
        <h2 className="typography typography--h4">CSS Classes</h2>
        <p className="typography typography--body-sm" style={{color:'var(--muted)'}}>
          HeroUI follows BEM so every slot and variant is reachable from your own stylesheet.
        </p>
        <div style={{display:'flex',flexWrap:'wrap',gap:8}}>
          {classes.map(c => <code key={c} className="kbd" style={{fontFamily:'var(--font-mono)',height:26,paddingInline:8}}>{c}</code>)}
        </div>
      </section>

      <section style={{display:'flex',flexDirection:'column',gap:10}}>
        <h2 className="typography typography--h4">API Reference</h2>
        <div className="table-container">
          <table className="table">
            <thead><tr>
              <th className="table__header-cell">Prop</th>
              <th className="table__header-cell">Type</th>
              <th className="table__header-cell">Default</th>
            </tr></thead>
            <tbody>
              {props.map(p => (
                <tr key={p[0]} className="table__row">
                  <td className="table__cell" style={{fontFamily:'var(--font-mono)',fontSize:13}}>{p[0]}</td>
                  <td className="table__cell" style={{fontFamily:'var(--font-mono)',fontSize:12.5,color:'var(--muted)'}}>{p[1]}</td>
                  <td className="table__cell" style={{fontFamily:'var(--font-mono)',fontSize:12.5,color:'var(--muted)'}}>{p[2]}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <Separator />
      <nav style={{display:'flex',gap:12}}>
        <Card variant="secondary" style={{flex:1,padding:'12px 16px'}}>
          <span style={{fontSize:12,color:'var(--muted)'}}>Previous</span>
          <div style={{fontWeight:500,fontSize:14}}>ButtonGroup</div>
        </Card>
        <Card variant="secondary" style={{flex:1,padding:'12px 16px',textAlign:'end'}}>
          <span style={{fontSize:12,color:'var(--muted)'}}>Next</span>
          <div style={{fontWeight:500,fontSize:14}}>Calendar</div>
        </Card>
      </nav>
    </article>
  );
}

Object.assign(window, {ComponentPage});
