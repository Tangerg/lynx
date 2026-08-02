import React from 'react';

/** Field with inline addons — icons, prefixes, trailing buttons. */
export function InputGroup({startContent, endContent, className = '', children, ...inputProps}) {
  return (
    <div className={['input-group', className].filter(Boolean).join(' ')}>
      {startContent && <span className="input-group__addon">{startContent}</span>}
      {children || <input className="input-group__input" {...inputProps} />}
      {endContent && <span className="input-group__addon">{endContent}</span>}
    </div>
  );
}
