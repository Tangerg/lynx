import type {HTMLAttributes, ReactNode} from 'react';

/** Keyboard-navigable row of tags with optional removal. */
export interface TagGroupProps extends HTMLAttributes<HTMLDivElement> {
  children?: ReactNode;
}
export declare function TagGroup(props: TagGroupProps): JSX.Element;
export declare namespace TagGroup {
  function Tag(props: {isSelected?: boolean; onRemove?: () => void; children?: ReactNode} & HTMLAttributes<HTMLSpanElement>): JSX.Element;
}
