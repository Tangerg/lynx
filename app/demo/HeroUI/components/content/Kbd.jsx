import React from 'react';

const GLYPH = {cmd: '\u2318', shift: '\u21E7', alt: '\u2325', ctrl: '\u2303', enter: '\u21B5', esc: 'esc', tab: '\u21E5', backspace: '\u232B', up: '\u2191', down: '\u2193', left: '\u2190', right: '\u2192'};

/** Keyboard key cap. */
export function Kbd({keys, className = '', children, ...rest}) {
  const list = keys ? (Array.isArray(keys) ? keys : [keys]) : null;
  return (
    <kbd className={['kbd', className].filter(Boolean).join(' ')} {...rest}>
      {list ? list.map(k => GLYPH[k] || k).join('') : children}
    </kbd>
  );
}
