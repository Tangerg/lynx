import type {FieldsetHTMLAttributes, ReactNode} from 'react';

/** Semantic grouping of related fields. */
export interface FieldsetProps extends FieldsetHTMLAttributes<HTMLFieldSetElement> {
  legend?: ReactNode;
  children?: ReactNode;
}
export declare function Fieldset(props: FieldsetProps): JSX.Element;
