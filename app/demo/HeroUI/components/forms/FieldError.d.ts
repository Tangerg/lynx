import type {HTMLAttributes, ReactNode} from 'react';

/** Inline validation message in `--danger`. */
export interface FieldErrorProps extends HTMLAttributes<HTMLParagraphElement> {
  children?: ReactNode;
}
export declare function FieldError(props: FieldErrorProps): JSX.Element | null;
