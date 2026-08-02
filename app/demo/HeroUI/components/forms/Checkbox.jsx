import React from 'react';
import {Icon} from '../content/Icon.jsx';

/** Checkbox with optional description. */
export function Checkbox({isSelected, defaultSelected = false, isIndeterminate = false, isInvalid = false, isDisabled = false, description, onChange, className = '', children, ...rest}) {
  const [inner, setInner] = React.useState(defaultSelected);
  const on = isSelected !== undefined ? isSelected : inner;
  const toggle = () => { if (isDisabled) return; if (isSelected === undefined) setInner(!on); onChange?.(!on); };
  return (
    <label className={['checkbox', className].filter(Boolean).join(' ')}
      data-selected={on || isIndeterminate ? 'true' : undefined}
      data-invalid={isInvalid ? 'true' : undefined}
      style={isDisabled ? {opacity: 'var(--disabled-opacity)', cursor: 'var(--cursor-disabled)'} : undefined} {...rest}>
      <input type="checkbox" checked={on} onChange={toggle} disabled={isDisabled}
        style={{position: 'absolute', opacity: 0, width: 1, height: 1}} />
      <span className="checkbox__box" aria-hidden="true">
        <Icon name={isIndeterminate ? 'minus' : 'check'} size={13} strokeWidth={3} />
      </span>
      <span className="checkbox__content">
        <span>{children}</span>
        {description && <span className="checkbox__description">{description}</span>}
      </span>
    </label>
  );
}
