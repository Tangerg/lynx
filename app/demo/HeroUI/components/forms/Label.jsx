import React from 'react';

/** Field label. Marks required fields with a danger asterisk. */
export function Label({isRequired = false, className = '', children, ...rest}) {
  return <label className={['label', className].filter(Boolean).join(' ')} {...rest}>{children}{isRequired && <span className="label__required">*</span>}</label>;
}
