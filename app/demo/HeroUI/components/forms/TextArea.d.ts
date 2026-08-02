import type {TextareaHTMLAttributes} from 'react';

/** Multi-line text field. */
export interface TextAreaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  /** @default 3 */
  rows?: number;
  /** @default false */
  isInvalid?: boolean;
}
export declare function TextArea(props: TextAreaProps): JSX.Element;
