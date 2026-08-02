import React from 'react';
import {Label} from './Label.jsx';

/** Single-choice group. Radio items are Radio children. */
export function RadioGroup({label, value, defaultValue, onChange, orientation = 'vertical', className = '', children, ...rest}) {
  const [inner, setInner] = React.useState(defaultValue);
  const current = value !== undefined ? value : inner;
  const select = v => { if (value === undefined) setInner(v); onChange?.(v); };
  return (
    <div role="radiogroup" className={['field', className].filter(Boolean).join(' ')} {...rest}>
      {label && <Label>{label}</Label>}
      <div className={['radio-group', orientation === 'horizontal' && 'radio-group--horizontal'].filter(Boolean).join(' ')}>
        {React.Children.map(children, child =>
          React.isValidElement(child)
            ? React.cloneElement(child, {isSelected: child.props.value === current, onSelect: () => select(child.props.value)})
            : child)}
      </div>
    </div>
  );
}
RadioGroup.Radio = function Radio({value, isSelected, onSelect, description, className = '', children, ...rest}) {
  return (
    <label className={['radio', className].filter(Boolean).join(' ')} data-selected={isSelected ? 'true' : undefined} {...rest}>
      <input type="radio" checked={!!isSelected} onChange={() => onSelect?.()} value={value}
        style={{position: 'absolute', opacity: 0, width: 1, height: 1}} />
      <span className="radio__dot" aria-hidden="true" />
      <span className="radio__content">
        <span>{children}</span>
        {description && <span className="radio__description">{description}</span>}
      </span>
    </label>
  );
};
