import type {InputHTMLAttributes, ReactNode} from 'react';

/**
 * Complete labelled text field.
 */
export interface TextFieldProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'size'> {
  label?: ReactNode;
  description?: ReactNode;
  /** Shown instead of `description` and puts the input in its invalid state. */
  errorMessage?: ReactNode;
  isRequired?: boolean;
  /** Supply your own control (Input, TextArea, InputGroup…). */
  children?: ReactNode;
}
export declare function TextField(props: TextFieldProps): JSX.Element;
