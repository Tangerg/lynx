import type {HTMLAttributes, ReactNode} from 'react';

/**
 * Non-overlay container built on `--surface` + `--surface-shadow`, 16px radius.
 * Every slot is a real element you can move, restyle or drop.
 *
 */
export interface CardProps extends HTMLAttributes<HTMLDivElement> {
  /** @default 'default' */
  variant?: 'transparent' | 'default' | 'secondary' | 'tertiary';
  children?: ReactNode;
}
export declare function Card(props: CardProps): JSX.Element;
export declare namespace Card {
  function Header(props: HTMLAttributes<HTMLDivElement>): JSX.Element;
  function Title(props: HTMLAttributes<HTMLHeadingElement>): JSX.Element;
  function Description(props: HTMLAttributes<HTMLParagraphElement>): JSX.Element;
  function Content(props: HTMLAttributes<HTMLDivElement>): JSX.Element;
  function Footer(props: HTMLAttributes<HTMLDivElement>): JSX.Element;
  function Media(props: HTMLAttributes<HTMLElement>): JSX.Element;
}
