import type {FormHTMLAttributes, ReactNode} from 'react';

/** Form element that stacks its fields consistently. */
export interface FormProps extends FormHTMLAttributes<HTMLFormElement> {
  children?: ReactNode;
}
export declare function Form(props: FormProps): JSX.Element;
