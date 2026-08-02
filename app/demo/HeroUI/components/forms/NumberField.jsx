import React from 'react';

/** Numeric input with steppers. */
export function NumberField({value, defaultValue = 0, step = 1, min, max, onChange, className = '', ...rest}) {
  const [inner, setInner] = React.useState(defaultValue);
  const val = value !== undefined ? value : inner;
  const set = n => {
    let next = n;
    if (min !== undefined) next = Math.max(min, next);
    if (max !== undefined) next = Math.min(max, next);
    if (value === undefined) setInner(next);
    onChange?.(next);
  };
  return (
    <div className={['number-field', className].filter(Boolean).join(' ')}>
      <input type="text" inputMode="numeric" className="input" value={val}
        onChange={e => set(Number(e.target.value) || 0)} style={{textAlign: 'end'}} {...rest} />
      <span className="number-field__steppers">
        <button type="button" className="number-field__stepper" aria-label="Increase" onClick={() => set(val + step)}>&#9650;</button>
        <button type="button" className="number-field__stepper" aria-label="Decrease" onClick={() => set(val - step)}>&#9660;</button>
      </span>
    </div>
  );
}
