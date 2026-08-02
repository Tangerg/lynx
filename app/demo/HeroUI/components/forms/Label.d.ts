import type {LabelHTMLAttributes, ReactNode} from 'react';

/** Accessible label for any field. */
export interface LabelProps extends LabelHTMLAttributes<HTMLLabelElement> {
  /** Appends a danger-coloured asterisk. @default false */
  isRequired?: boolean;
  children?: ReactNode;
}
export declare function Label(props: LabelProps): JSX.Element;
