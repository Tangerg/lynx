// Slot — a named shell region filled by plugin contributions.
//
// `<Slot name="app.sidebar"/>` renders every plugin component registered
// for that slot, ordered by `order ?? 100`. Each contribution is wrapped in
// its own PluginBoundary so one bad render doesn't take down the shell.
//
// By default the slot is *transparent* (renders as a Fragment) — important
// because most of our shell slots sit inside a CSS grid whose layout depends
// on the immediate children being the panels themselves. Pass `wrapper=true`
// if you want a real `<div data-slot=...>` (e.g. for hit-testing or zone
// styling).

import { Fragment } from "react";
import { useLayoutSlot } from "@/plugins/sdk";
import { PluginBoundary } from "./PluginBoundary";

type Props = {
  name: string;
  /** When true, wrap the slot contents in `<div data-slot={name} className={className}/>`. */
  wrapper?: boolean;
  /** className on the wrapping `<div>` — implies `wrapper=true`. */
  className?: string;
};

export function Slot({ name, wrapper, className }: Props) {
  const specs = useLayoutSlot(name);
  if (specs.length === 0) return null;

  const children = specs.map((spec) => {
    const Component = spec.component;
    const body = spec.className ? (
      <div className={spec.className}>
        <Component />
      </div>
    ) : (
      <Component />
    );
    return (
      <PluginBoundary
        key={spec.id}
        plugin={`layout:${name}:${spec.id}`}
        label={`${name} slot`}
      >
        {body}
      </PluginBoundary>
    );
  });

  if (wrapper || className) {
    return (
      <div data-slot={name} className={className}>
        {children}
      </div>
    );
  }
  return <Fragment>{children}</Fragment>;
}
