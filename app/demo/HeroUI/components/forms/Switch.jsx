import React from 'react';

/** On/off toggle. */
export function Switch({isSelected, defaultSelected = false, isDisabled = false, onChange, labelPosition = 'end', className = '', children, ...rest}) {
  const [inner, setInner] = React.useState(defaultSelected);
  const on = isSelected !== undefined ? isSelected : inner;
  const toggle = () => { if (isDisabled) return; if (isSelected === undefined) setInner(!on); onChange?.(!on); };
  const track = <span className="switch__track" aria-hidden="true"><span className="switch__thumb" /></span>;
  return (
    <label className={['switch', className].filter(Boolean).join(' ')} data-selected={on ? 'true' : undefined}
      style={isDisabled ? {opacity: 'var(--disabled-opacity)', cursor: 'var(--cursor-disabled)'} : undefined} {...rest}>
      <input type="checkbox" role="switch" checked={on} onChange={toggle} disabled={isDisabled}
        style={{position: 'absolute', opacity: 0, width: 1, height: 1}} />
      {labelPosition === 'start' && <span>{children}</span>}
      {track}
      {labelPosition === 'end' && <span>{children}</span>}
    </label>
  );
}
