import type {InputHTMLAttributes} from 'react';

/**
 * Single-line text field. Borderless by default: `--field-background` + `--field-shadow`,
 * 12px radius, 40px tall.
 */
export interface InputProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'size'> {
  /** @default 'md' */
  size?: 'sm' | 'md' | 'lg';
  /** @default false */
  isInvalid?: boolean;
}
export declare function Input(props: InputProps): JSX.Element;
