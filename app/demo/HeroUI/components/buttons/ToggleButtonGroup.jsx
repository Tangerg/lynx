import React from 'react';

/** Segmented switch: a tray of ToggleButtons where one (or many) stay lit. */
export function ToggleButtonGroup({className = '', children, ...rest}) {
  return <div role="group" className={['toggle-button-group', className].filter(Boolean).join(' ')} {...rest}>{children}</div>;
}
