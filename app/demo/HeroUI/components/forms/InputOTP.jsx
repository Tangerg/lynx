import React from 'react';

/** One-time-code entry. One slot per digit. */
export function InputOTP({length = 6, value = '', activeIndex = 0, className = '', ...rest}) {
  return (
    <div className={['input-otp', className].filter(Boolean).join(' ')} {...rest}>
      {Array.from({length}, (_, i) => (
        <span key={i} className="input-otp__slot" data-active={i === activeIndex ? 'true' : undefined}>
          {value[i] || (i === activeIndex ? <span className="input-otp__caret" /> : null)}
        </span>
      ))}
    </div>
  );
}
