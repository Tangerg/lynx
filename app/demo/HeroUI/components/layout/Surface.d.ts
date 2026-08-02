import type {HTMLAttributes, ReactNode} from 'react';

/** Themed background container with no built-in padding or slots. */
export interface SurfaceProps extends HTMLAttributes<HTMLDivElement> {
  /** @default 'default' */
  variant?: 'transparent' | 'default' | 'secondary' | 'tertiary';
  children?: ReactNode;
}
export declare function Surface(props: SurfaceProps): JSX.Element;
