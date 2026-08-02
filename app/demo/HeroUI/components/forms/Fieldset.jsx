import React from 'react';

/** Grouped fields with a legend. */
export function Fieldset({legend, className = '', children, ...rest}) {
  return (
    <fieldset className={['fieldset', className].filter(Boolean).join(' ')} {...rest}>
      {legend && <legend className="fieldset__legend">{legend}</legend>}
      {children}
    </fieldset>
  );
}
