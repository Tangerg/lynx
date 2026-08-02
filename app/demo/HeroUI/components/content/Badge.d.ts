import type {HTMLAttributes, ReactNode} from 'react';

/** Overlays a count or status dot on the element it wraps. */
export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  /** Text/number shown in the pill. */
  content?: ReactNode;
  /** @default 'accent' */
  color?: 'accent' | 'danger' | 'success';
  /** Drop the label and show a 10px dot. @default false */
  dot?: boolean;
  /** @default false */
  hidden?: boolean;
  children?: ReactNode;
}
export declare function Badge(props: BadgeProps): JSX.Element;
