import React from 'react';

/** Vertical stack of fields, 16px apart. */
export function Form({className = '', children, ...rest}) {
  return <form className={['form', className].filter(Boolean).join(' ')} {...rest}>{children}</form>;
}
