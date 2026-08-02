import type {HTMLAttributes, ReactNode} from 'react';

/** Semantic typography primitive. (v2's `Text`.) */
export interface TypographyProps extends HTMLAttributes<HTMLElement> {
  /** @default 'body' */
  type?: 'h1' | 'h2' | 'h3' | 'h4' | 'h5' | 'h6' | 'body' | 'body-sm' | 'body-xs' | 'code';
  /** @default 'start' */
  align?: 'start' | 'center' | 'end' | 'justify';
  /** @default 'default' */
  color?: 'default' | 'muted';
  weight?: 'normal' | 'medium' | 'semibold' | 'bold';
  /** One line with an ellipsis. */
  truncate?: boolean;
  /** Override the rendered element. */
  as?: keyof JSX.IntrinsicElements;
  children?: ReactNode;
}
export declare function Typography(props: TypographyProps): JSX.Element;
export declare namespace Typography {
  function Heading(props: TypographyProps & {level?: 1 | 2 | 3 | 4 | 5 | 6}): JSX.Element;
  function Paragraph(props: TypographyProps & {size?: 'base' | 'sm' | 'xs'}): JSX.Element;
  function Code(props: TypographyProps): JSX.Element;
  function Prose(props: HTMLAttributes<HTMLDivElement>): JSX.Element;
}
