import type {InputHTMLAttributes, ReactNode} from 'react';

/** Input wrapper that hosts leading/trailing content inside the field. */
export interface InputGroupProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'size' | 'children'> {
  startContent?: ReactNode;
  endContent?: ReactNode;
  children?: ReactNode;
}
export declare function InputGroup(props: InputGroupProps): JSX.Element;
