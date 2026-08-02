import type {AnchorHTMLAttributes, ReactNode} from 'react';

/** Navigational text link. */
export interface LinkProps extends AnchorHTMLAttributes<HTMLAnchorElement> {
  /** Defaults to `--link` (the body colour). */
  variant?: 'accent' | 'muted';
  /** Opens in a new tab and appends the ↗ glyph. @default false */
  isExternal?: boolean;
  children?: ReactNode;
}
export declare function Link(props: LinkProps): JSX.Element;
