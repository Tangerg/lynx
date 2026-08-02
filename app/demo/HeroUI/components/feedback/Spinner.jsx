import React from 'react';

/** Indeterminate loading indicator. */
export function Spinner({size = 'md', label, className = '', ...rest}) {
  return <span data-slot="spinner" role="status" aria-label={label || 'Loading'}
    className={['spinner', size !== 'md' && `spinner--${size}`, className].filter(Boolean).join(' ')} {...rest} />;
}
