import type {HTMLAttributes, TableHTMLAttributes, ReactNode} from 'react';

/**
 * Tabular data on a surface container.
 */
export interface TableProps extends TableHTMLAttributes<HTMLTableElement> {
  children?: ReactNode;
}
export declare function Table(props: TableProps): JSX.Element;
export declare namespace Table {
  function Head(props: HTMLAttributes<HTMLTableSectionElement>): JSX.Element;
  function HeaderCell(props: {isSortable?: boolean; children?: ReactNode} & HTMLAttributes<HTMLTableCellElement>): JSX.Element;
  function Body(props: HTMLAttributes<HTMLTableSectionElement>): JSX.Element;
  function Row(props: {isSelected?: boolean; children?: ReactNode} & HTMLAttributes<HTMLTableRowElement>): JSX.Element;
  function Cell(props: {numeric?: boolean; children?: ReactNode} & HTMLAttributes<HTMLTableCellElement>): JSX.Element;
  function Footer(props: HTMLAttributes<HTMLDivElement>): JSX.Element;
}
