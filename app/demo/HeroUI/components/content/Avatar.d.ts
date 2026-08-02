import type {HTMLAttributes, ReactNode} from 'react';

/** User or entity image with an initials fallback. */
export interface AvatarProps extends HTMLAttributes<HTMLSpanElement> {
  src?: string;
  alt?: string;
  /** Used to derive initials when `src` is absent. */
  name?: string;
  /** @default 'md' */
  size?: 'sm' | 'md' | 'lg';
  /** @default 'circle' */
  shape?: 'circle' | 'square';
  children?: ReactNode;
}
export declare function Avatar(props: AvatarProps): JSX.Element;
export declare namespace Avatar {
  function Group(props: HTMLAttributes<HTMLSpanElement>): JSX.Element;
}
