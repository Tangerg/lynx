import React from 'react';

/** Muted helper text under a field. */
export function Description({className = '', children, ...rest}) {
  return <p className={['description', className].filter(Boolean).join(' ')} {...rest}>{children}</p>;
}
