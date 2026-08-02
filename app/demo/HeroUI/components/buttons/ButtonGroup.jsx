import React from 'react';

/** Segmented run of Buttons — inner corners flatten unless `gap` is set. */
export function ButtonGroup({gap = false, className = '', children, ...rest}) {
  return (
    <div role="group" className={['button-group', gap && 'button-group--gap', className].filter(Boolean).join(' ')} {...rest}>
      {children}
    </div>
  );
}
