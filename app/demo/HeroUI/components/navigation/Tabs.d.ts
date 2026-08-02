import type {HTMLAttributes, ReactNode} from 'react';

/**
 * Switches between sibling views.
 */
export interface TabsProps extends HTMLAttributes<HTMLDivElement> {
  /** @default 'segmented' */
  variant?: 'segmented' | 'underline';
  selectedKey?: string;
  defaultSelectedKey?: string;
  onSelectionChange?: (key: string) => void;
  /** @default 'horizontal' */
  orientation?: 'horizontal' | 'vertical';
  children?: ReactNode;
}
export declare function Tabs(props: TabsProps): JSX.Element;
export declare namespace Tabs {
  function ListContainer(props: HTMLAttributes<HTMLDivElement>): JSX.Element;
  function Tab(props: {id: string; isDisabled?: boolean; children?: ReactNode} & HTMLAttributes<HTMLButtonElement>): JSX.Element;
  function Panel(props: {id: string; children?: ReactNode} & HTMLAttributes<HTMLDivElement>): JSX.Element;
}
