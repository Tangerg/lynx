import React from 'react';

/** Validation message. Renders nothing when there is no message. */
export function FieldError({className = '', children, ...rest}) {
  if (!children) return null;
  return <p role="alert" className={['field-error', className].filter(Boolean).join(' ')} {...rest}>{children}</p>;
}
