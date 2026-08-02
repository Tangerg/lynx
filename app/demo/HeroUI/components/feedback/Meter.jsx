import React from 'react';

/** Static measurement within a range (quota, disk, score). */
export function Meter({value = 0, max = 100, label, showValue = true, color, className = '', ...rest}) {
  const pct = Math.min(100, Math.max(0, (value / max) * 100));
  return (
    <div className={['meter', color && `meter--${color}`, className].filter(Boolean).join(' ')} role="meter" aria-valuenow={value} aria-valuemax={max} {...rest}>
      {(label || showValue) && <div className="meter__header"><span>{label}</span>{showValue && <span>{value} / {max}</span>}</div>}
      <div className="meter__track"><div className="meter__fill" style={{width: `${pct}%`}} /></div>
    </div>
  );
}
