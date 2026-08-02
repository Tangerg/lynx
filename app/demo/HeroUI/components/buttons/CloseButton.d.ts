import type {ButtonHTMLAttributes} from 'react';

/** Dismiss affordance for modals, drawers, toasts and banners. */
export interface CloseButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /** @default 'md' */
  size?: 'sm' | 'md' | 'lg';
  /** Accessible name. @default 'Close' */
  label?: string;
}
export declare function CloseButton(props: CloseButtonProps): JSX.Element;
