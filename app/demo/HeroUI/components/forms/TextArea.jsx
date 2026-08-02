import React from 'react';

/** Multi-line input. Vertical resize only. */
export function TextArea({rows = 3, isInvalid = false, className = '', ...rest}) {
  return <textarea rows={rows} className={['input', 'text-area', isInvalid && 'input--invalid', className].filter(Boolean).join(' ')} aria-invalid={isInvalid || undefined} {...rest} />;
}
