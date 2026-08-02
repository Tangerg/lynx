import type {HTMLAttributes, ReactNode} from 'react';

/**
 * Focus-trapping dialog centred on a blurred scrim.
 */
export interface ModalProps extends HTMLAttributes<HTMLDivElement> {
  /** @default true */
  isOpen?: boolean;
  onClose?: () => void;
  /** @default 'md' */
  size?: 'sm' | 'md' | 'lg';
  children?: ReactNode;
}
export declare function Modal(props: ModalProps): JSX.Element | null;
export declare namespace Modal {
  function Header(props: {title?: ReactNode; description?: ReactNode; onClose?: () => void} & HTMLAttributes<HTMLDivElement>): JSX.Element;
  function Content(props: HTMLAttributes<HTMLDivElement>): JSX.Element;
  function Footer(props: HTMLAttributes<HTMLDivElement>): JSX.Element;
}
