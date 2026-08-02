import React from 'react';
import {Icon} from '../content/Icon.jsx';
import {DateField} from './DateField.jsx';
import {Calendar} from './Calendar.jsx';

/** DateField plus a calendar popover. */
export function DatePicker({value, onSelect, className = '', ...rest}) {
  const [open, setOpen] = React.useState(false);
  return (
    <span style={{position: 'relative', display: 'inline-block'}} className={className} {...rest}>
      <span className="date-picker">
        <DateField value={value} />
        <button type="button" className="date-picker__trigger" aria-label="Open calendar" onClick={() => setOpen(!open)}>
          <Icon name="calendar" />
        </button>
      </span>
      {open && (
        <span style={{position: 'absolute', top: '100%', insetInlineStart: 0, marginTop: 6, zIndex: 40, display: 'block'}}>
          <Calendar onSelect={d => { onSelect?.(d); setOpen(false); }} />
        </span>
      )}
    </span>
  );
}
