import React from 'react';

/** Determinate or indeterminate horizontal progress. */
export function ProgressBar({value = 0, max = 100, label, showValue = false, color, isIndeterminate = false, className = '', ...rest}) {
  const pct = Math.min(100, Math.max(0, (value / max) * 100));
  return (
    <div className={['progress-bar', color && `progress-bar--${color}`, isIndeterminate && 'progress-bar--indeterminate', className].filter(Boolean).join(' ')}
      role="progressbar" aria-valuenow={isIndeterminate ? undefined : value} aria-valuemax={max} {...rest}>
      {(label || showValue) && (
        <div className="progress-bar__header"><span>{label}</span>{showValue && <span>{Math.round(pct)}%</span>}</div>
      )}
      <div className="progress-bar__track"><div className="progress-bar__fill" style={isIndeterminate ? undefined : {width: `${pct}%`}} /></div>
    </div>
  );
}
