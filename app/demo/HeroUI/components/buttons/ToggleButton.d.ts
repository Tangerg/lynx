import type {ButtonHTMLAttributes, ReactNode} from 'react';

/** A Button with an on/off state — bold, italic, "show archived". */
export interface ToggleButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'disabled'> {
  /** @default false */
  isSelected?: boolean;
  /** @default 'md' */
  size?: 'sm' | 'md' | 'lg';
  /** @default false */
  isIconOnly?: boolean;
  /** @default false */
  isDisabled?: boolean;
  children?: ReactNode;
}
export declare function ToggleButton(props: ToggleButtonProps): JSX.Element;
