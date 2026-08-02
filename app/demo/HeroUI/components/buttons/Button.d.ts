import type {ButtonHTMLAttributes, ReactNode} from 'react';

/**
 * Primary action control. Pill-shaped, token-driven; every variant only swaps
 * the `--button-bg` / `--button-fg` custom properties.
 *
 */
export interface ButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'disabled'> {
  /** @default 'primary' */
  variant?: 'primary' | 'secondary' | 'tertiary' | 'ghost' | 'outline' | 'danger' | 'danger-soft';
  /** @default 'md' */
  size?: 'sm' | 'md' | 'lg';
  /** Square button holding a single glyph. @default false */
  isIconOnly?: boolean;
  /** @default false */
  fullWidth?: boolean;
  /** @default false */
  isDisabled?: boolean;
  /** Swaps the leading icon for a spinner and blocks interaction. @default false */
  isPending?: boolean;
  /** Force the pressed look (for docs/specimens). @default false */
  isPressed?: boolean;
  startIcon?: ReactNode;
  endIcon?: ReactNode;
  children?: ReactNode;
}
export declare function Button(props: ButtonProps): JSX.Element;
