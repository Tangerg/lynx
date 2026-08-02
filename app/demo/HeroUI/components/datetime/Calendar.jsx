import React from 'react';
import {Icon} from '../content/Icon.jsx';

const WEEKDAYS = ['Su', 'Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa'];
const MONTHS = ['January','February','March','April','May','June','July','August','September','October','November','December'];

/** Month grid. Set `rangeEnd` for a RangeCalendar. */
export function Calendar({month, year, selected, rangeEnd, onSelect, className = '', ...rest}) {
  const today = new Date();
  const m = month ?? today.getMonth();
  const y = year ?? today.getFullYear();
  const first = new Date(y, m, 1).getDay();
  const days = new Date(y, m + 1, 0).getDate();
  const cells = [];
  for (let i = 0; i < first; i++) cells.push(null);
  for (let d = 1; d <= days; d++) cells.push(d);
  const inRange = d => selected && rangeEnd && d > selected && d < rangeEnd;
  return (
    <div className={['calendar', className].filter(Boolean).join(' ')} {...rest}>
      <div className="calendar__header">
        <button type="button" className="date-picker__trigger" aria-label="Previous month"><Icon name="chevron-left" /></button>
        <span className="calendar__heading">{MONTHS[m]} {y}</span>
        <button type="button" className="date-picker__trigger" aria-label="Next month"><Icon name="chevron-right" /></button>
      </div>
      <div className="calendar__grid">
        {WEEKDAYS.map(w => <span key={w} className="calendar__weekday">{w}</span>)}
        {cells.map((d, i) => d === null
          ? <span key={`b${i}`} />
          : <button key={d} type="button" className="calendar__cell"
              data-today={d === today.getDate() && m === today.getMonth() && y === today.getFullYear() ? 'true' : undefined}
              data-selected={d === selected || d === rangeEnd ? 'true' : undefined}
              data-in-range={inRange(d) ? 'true' : undefined}
              onClick={() => onSelect?.(d)}>{d}</button>)}
      </div>
    </div>
  );
}
