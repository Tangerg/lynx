Switch between sibling views. Segmented by default (pill tray, selected tab lifts onto `--segment`); `variant="underline"` for docs-style bars.

```jsx
<Tabs defaultSelectedKey="preview">
  <Tabs.ListContainer>
    <Tabs.Tab id="preview">Preview</Tabs.Tab>
    <Tabs.Tab id="code">Code</Tabs.Tab>
  </Tabs.ListContainer>
  <Tabs.Panel id="preview">…</Tabs.Panel>
  <Tabs.Panel id="code">…</Tabs.Panel>
</Tabs>
```

Note the v3 rename: `TabListWrapper` → `Tabs.ListContainer`.
