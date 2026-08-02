import type {HTMLAttributes, ReactNode} from 'react';

/** Scrollable list of selectable options on an overlay surface. */
export interface ListBoxProps extends HTMLAttributes<HTMLDivElement> {
  children?: ReactNode;
}
export declare function ListBox(props: ListBoxProps): JSX.Element;
export declare namespace ListBox {
  function Item(props: {isSelected?: boolean; isDisabled?: boolean; startContent?: ReactNode; description?: ReactNode; showCheck?: boolean; children?: ReactNode} & HTMLAttributes<HTMLDivElement>): JSX.Element;
  function Section(props: {label?: ReactNode; children?: ReactNode}): JSX.Element;
}
