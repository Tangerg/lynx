import type {HTMLAttributes, ReactNode} from 'react';

/** Labelled set of related Checkboxes. */
export interface CheckboxGroupProps extends HTMLAttributes<HTMLDivElement> {
  label?: ReactNode;
  description?: ReactNode;
  /** @default 'vertical' */
  orientation?: 'vertical' | 'horizontal';
  children?: ReactNode;
}
export declare function CheckboxGroup(props: CheckboxGroupProps): JSX.Element;
