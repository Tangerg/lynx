import React from 'react';
import {Icon} from '../content/Icon.jsx';
import {ListBox} from './ListBox.jsx';

/** Trigger + popover list. Options are `{value,label,description}`. */
export function Select({options = [], value, defaultValue, placeholder = 'Select…', onChange, className = '', ...rest}) {
  const [open, setOpen] = React.useState(false);
  const [inner, setInner] = React.useState(defaultValue);
  const current = value !== undefined ? value : inner;
  const selected = options.find(o => o.value === current);
  const pick = v => { if (value === undefined) setInner(v); onChange?.(v); setOpen(false); };
  return (
    <div style={{position: 'relative'}} className={className} {...rest}>
      <button type="button" className="select__trigger" data-open={open ? 'true' : undefined}
        aria-haspopup="listbox" aria-expanded={open} onClick={() => setOpen(!open)}>
        <span className={selected ? '' : 'select__value--placeholder'}>{selected ? selected.label : placeholder}</span>
        <Icon name="chevron-down" className="select__chevron" />
      </button>
      {open && (
        <div style={{position: 'absolute', insetInlineStart: 0, insetInlineEnd: 0, marginTop: 6, zIndex: 40}}>
          <ListBox>
            {options.map(o => (
              <ListBox.Item key={o.value} isSelected={o.value === current} showCheck
                description={o.description} onClick={() => pick(o.value)}>{o.label}</ListBox.Item>
            ))}
          </ListBox>
        </div>
      )}
    </div>
  );
}
