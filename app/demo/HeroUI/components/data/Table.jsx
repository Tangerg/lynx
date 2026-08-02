import React from 'react';

/** Compound table. Table.Head / Row / HeaderCell / Body / Cell / Footer. */
export function Table({className = '', children, ...rest}) {
  return (
    <div className={['table-container', className].filter(Boolean).join(' ')}>
      <table className="table" {...rest}>{children}</table>
    </div>
  );
}
Table.Head = function TableHead({children, ...rest}) { return <thead {...rest}><tr>{children}</tr></thead>; };
Table.HeaderCell = function TableHeaderCell({isSortable, className = '', children, ...rest}) {
  return <th className={['table__header-cell', className].filter(Boolean).join(' ')} data-sortable={isSortable ? 'true' : undefined} {...rest}>{children}</th>;
};
Table.Body = function TableBody(props) { return <tbody {...props} />; };
Table.Row = function TableRow({isSelected, className = '', children, ...rest}) {
  return <tr className={['table__row', className].filter(Boolean).join(' ')} data-selected={isSelected ? 'true' : undefined} {...rest}>{children}</tr>;
};
Table.Cell = function TableCell({numeric, className = '', children, ...rest}) {
  return <td className={['table__cell', numeric && 'table__cell--numeric', className].filter(Boolean).join(' ')} {...rest}>{children}</td>;
};
Table.Footer = function TableFooter({className = '', children, ...rest}) {
  return <div className={['table__footer', className].filter(Boolean).join(' ')} {...rest}>{children}</div>;
};
