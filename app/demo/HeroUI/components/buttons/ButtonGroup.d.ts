import type {HTMLAttributes, ReactNode} from 'react';

/** Joins Buttons into one segmented control. */
export interface ButtonGroupProps extends HTMLAttributes<HTMLDivElement> {
  /** Keep each button fully rounded and space them 8px apart. @default false */
  gap?: boolean;
  children?: ReactNode;
}
export declare function ButtonGroup(props: ButtonGroupProps): JSX.Element;
