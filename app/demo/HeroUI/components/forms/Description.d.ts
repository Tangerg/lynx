import type {HTMLAttributes, ReactNode} from 'react';

/** Helper copy shown beneath a field. */
export interface DescriptionProps extends HTMLAttributes<HTMLParagraphElement> {
  children?: ReactNode;
}
export declare function Description(props: DescriptionProps): JSX.Element;
