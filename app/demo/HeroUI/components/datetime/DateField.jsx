import React from 'react';

/** Segmented date entry (MM / DD / YYYY). */
export function DateField({value, className = '', ...rest}) {
  const [mm, dd, yyyy] = (value || '').split('/');
  const seg = (v, ph) => (
    <span className="date-field__segment" data-placeholder={v ? undefined : 'true'} tabIndex={0}>{v || ph}</span>
  );
  return (
    <div className={['date-field', className].filter(Boolean).join(' ')} {...rest}>
      {seg(mm, 'mm')}<span className="date-field__literal">/</span>
      {seg(dd, 'dd')}<span className="date-field__literal">/</span>
      {seg(yyyy, 'yyyy')}
    </div>
  );
}
