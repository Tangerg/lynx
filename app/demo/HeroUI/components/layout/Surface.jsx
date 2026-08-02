import React from 'react';

/** Raw themed container — the primitive Card is built from. */
export function Surface({variant = 'default', className = '', children, ...rest}) {
  return <div className={['surface', variant !== 'default' && `surface--${variant}`, className].filter(Boolean).join(' ')} {...rest}>{children}</div>;
}
