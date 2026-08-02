import type {HTMLAttributes, ReactNode} from 'react';

/** Menu of actions hung off a trigger element. */
export interface DropdownProps extends HTMLAttributes<HTMLDivElement> {
  /** The clickable element (usually a Button). */
  trigger?: ReactNode;
  /** @default 'start' */
  align?: 'start' | 'end';
  children?: ReactNode;
}
export declare function Dropdown(props: DropdownProps): JSX.Element;
export declare namespace Dropdown {
  function Item(props: {danger?: boolean; shortcut?: ReactNode; children?: ReactNode} & HTMLAttributes<HTMLDivElement>): JSX.Element;
  function Separator(): JSX.Element;
}
