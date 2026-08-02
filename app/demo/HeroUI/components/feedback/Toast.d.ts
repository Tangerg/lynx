import type {HTMLAttributes, ReactNode} from 'react';

/** Transient notification on an overlay surface. */
export interface ToastProps extends HTMLAttributes<HTMLDivElement> {
  title?: ReactNode;
  icon?: ReactNode;
  /** Trailing action, usually a ghost Button. */
  action?: ReactNode;
  onClose?: () => void;
  children?: ReactNode;
}
export declare function Toast(props: ToastProps): JSX.Element;
export declare namespace Toast {
  function Region(props: HTMLAttributes<HTMLDivElement>): JSX.Element;
}
