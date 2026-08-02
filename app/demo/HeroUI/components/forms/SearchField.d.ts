import type {InputHTMLAttributes} from 'react';

/** Search input with the magnifier inset. */
export interface SearchFieldProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'size' | 'type'> {}
export declare function SearchField(props: SearchFieldProps): JSX.Element;
