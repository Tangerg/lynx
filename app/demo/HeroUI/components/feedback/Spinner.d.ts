import type {HTMLAttributes} from 'react';

/** Spinning ring for indeterminate waits. */
export interface SpinnerProps extends HTMLAttributes<HTMLSpanElement> {
  /** @default 'md' */
  size?: 'sm' | 'md' | 'lg';
  /** Accessible label. @default 'Loading' */
  label?: string;
}
export declare function Spinner(props: SpinnerProps): JSX.Element;
