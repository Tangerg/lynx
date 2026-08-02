import type {HTMLAttributes, ReactNode} from 'react';

/** Panel that slides in from an edge, inset 8px with a 24px radius. */
export interface DrawerProps extends HTMLAttributes<HTMLElement> {
  /** @default true */
  isOpen?: boolean;
  onClose?: () => void;
  /** @default 'right' */
  placement?: 'right' | 'bottom';
  children?: ReactNode;
}
export declare function Drawer(props: DrawerProps): JSX.Element | null;
export declare namespace Drawer {
  function Header(props: HTMLAttributes<HTMLDivElement>): JSX.Element;
  function Content(props: HTMLAttributes<HTMLDivElement>): JSX.Element;
  function Footer(props: HTMLAttributes<HTMLDivElement>): JSX.Element;
}
