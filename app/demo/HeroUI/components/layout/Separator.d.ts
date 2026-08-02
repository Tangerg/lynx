import type {HTMLAttributes} from 'react';

/** 1px rule between sections. (v2's `Divider`.) */
export interface SeparatorProps extends HTMLAttributes<HTMLDivElement> {
  /** @default 'horizontal' */
  orientation?: 'horizontal' | 'vertical';
  /** Darker steps for busier surfaces. */
  level?: 'secondary' | 'tertiary';
}
export declare function Separator(props: SeparatorProps): JSX.Element;
