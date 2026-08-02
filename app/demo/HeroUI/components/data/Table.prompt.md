Tabular data inside a surface container — muted header band, hairline row rules, hover tint.

```jsx
<Table>
  <Table.Head><Table.HeaderCell>Project</Table.HeaderCell><Table.HeaderCell>Status</Table.HeaderCell></Table.Head>
  <Table.Body>
    <Table.Row><Table.Cell>heroui.com</Table.Cell><Table.Cell><Chip color="success" dot>Live</Chip></Table.Cell></Table.Row>
  </Table.Body>
</Table>
```

Put numbers in `<Table.Cell numeric>` for tabular figures and end alignment.
