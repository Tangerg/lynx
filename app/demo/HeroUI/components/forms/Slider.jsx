import React from 'react';

/** Single-value slider with an optional readout. */
export function Slider({label, value, defaultValue = 50, min = 0, max = 100, step = 1, formatValue, onChange, className = '', ...rest}) {
  const [inner, setInner] = React.useState(defaultValue);
  const val = value !== undefined ? value : inner;
  const pct = ((val - min) / (max - min)) * 100;
  const set = n => { if (value === undefined) setInner(n); onChange?.(n); };
  return (
    <div className={['slider', className].filter(Boolean).join(' ')} {...rest}>
      {(label || formatValue) && (
        <div className="slider__header">
          <span>{label}</span>
          <span className="slider__output">{formatValue ? formatValue(val) : val}</span>
        </div>
      )}
      <div style={{position: 'relative', display: 'flex', alignItems: 'center'}}>
        <div className="slider__track" style={{width: '100%'}}>
          <div className="slider__fill" style={{width: `${pct}%`}} />
          <span className="slider__thumb" style={{insetInlineStart: `${pct}%`}} />
        </div>
        <input type="range" min={min} max={max} step={step} value={val}
          onChange={e => set(Number(e.target.value))} aria-label={typeof label === 'string' ? label : 'Slider'}
          style={{position: 'absolute', inset: 0, width: '100%', opacity: 0, cursor: 'pointer'}} />
      </div>
    </div>
  );
}
