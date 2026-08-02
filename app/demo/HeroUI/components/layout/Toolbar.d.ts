import type {HTMLAttributes, ReactNode} from 'react';

/** Grouped action bar — editor toolbars, canvas controls. */
export interface ToolbarProps extends HTMLAttributes<HTMLDivElement> {
  /** @default 'horizontal' */
  orientation?: 'horizontal' | 'vertical';
  children?: ReactNode;
}
export declare function Toolbar(props: ToolbarProps): JSX.Element;
export declare namespace Toolbar {
  function Separator(props: HTMLAttributes<HTMLDivElement>): JSX.Element;
}
