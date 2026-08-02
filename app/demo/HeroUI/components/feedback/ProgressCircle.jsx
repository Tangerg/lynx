import React from 'react';

/** Ring progress with an optional centred readout. */
export function ProgressCircle({value = 0, max = 100, size = 48, thickness = 4, showValue = false, className = '', ...rest}) {
  const pct = Math.min(100, Math.max(0, (value / max) * 100));
  const r = (size - thickness) / 2;
  const c = 2 * Math.PI * r;
  return (
    <span className={['progress-circle', className].filter(Boolean).join(' ')} role="progressbar" aria-valuenow={value} aria-valuemax={max} {...rest}>
      <svg className="progress-circle__svg" width={size} height={size}>
        <circle className="progress-circle__track" cx={size / 2} cy={size / 2} r={r} strokeWidth={thickness} />
        <circle className="progress-circle__fill" cx={size / 2} cy={size / 2} r={r} strokeWidth={thickness}
          strokeDasharray={c} strokeDashoffset={c - (c * pct) / 100} />
      </svg>
      {showValue && <span className="progress-circle__label">{Math.round(pct)}%</span>}
    </span>
  );
}
