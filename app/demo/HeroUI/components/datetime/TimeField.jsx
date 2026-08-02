import React from 'react';

/** Segmented time entry (HH : MM AM/PM). */
export function TimeField({value = '', className = '', ...rest}) {
  const [time, meridiem] = value.split(' ');
  const [hh, mm] = (time || '').split(':');
  const seg = (v, ph) => <span className="date-field__segment" data-placeholder={v ? undefined : 'true'} tabIndex={0}>{v || ph}</span>;
  return (
    <div className={['date-field', className].filter(Boolean).join(' ')} {...rest}>
      {seg(hh, 'hh')}<span className="date-field__literal">:</span>{seg(mm, 'mm')}{seg(meridiem, 'AM')}
    </div>
  );
}
