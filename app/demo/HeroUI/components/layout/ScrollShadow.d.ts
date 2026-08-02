import type {HTMLAttributes, ReactNode} from 'react';

/** Scrollable box whose overflowing edges fade out via a CSS mask. */
export interface ScrollShadowProps extends HTMLAttributes<HTMLDivElement> {
  /** @default 'vertical' */
  orientation?: 'vertical' | 'horizontal';
  /** Fade length in px. @default 40 */
  size?: number;
  children?: ReactNode;
}
export declare function ScrollShadow(props: ScrollShadowProps): JSX.Element;
