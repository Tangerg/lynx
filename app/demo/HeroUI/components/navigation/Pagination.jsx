import React from 'react';
import {Icon} from '../content/Icon.jsx';

/** Page selector with ellipsis collapsing. */
export function Pagination({total = 1, page = 1, siblings = 1, onChange, className = '', ...rest}) {
  const pages = [];
  for (let i = 1; i <= total; i++) {
    if (i === 1 || i === total || Math.abs(i - page) <= siblings) pages.push(i);
    else if (pages[pages.length - 1] !== '…') pages.push('…');
  }
  return (
    <nav className={['pagination', className].filter(Boolean).join(' ')} aria-label="Pagination" {...rest}>
      <button type="button" className="pagination__item" aria-label="Previous"
        data-disabled={page === 1 ? 'true' : undefined} disabled={page === 1}
        onClick={() => onChange?.(page - 1)}><Icon name="chevron-left" /></button>
      {pages.map((p, i) => p === '…'
        ? <span key={`e${i}`} className="pagination__ellipsis">…</span>
        : <button key={p} type="button" className="pagination__item"
            data-selected={p === page ? 'true' : undefined} onClick={() => onChange?.(p)}>{p}</button>)}
      <button type="button" className="pagination__item" aria-label="Next"
        data-disabled={page === total ? 'true' : undefined} disabled={page === total}
        onClick={() => onChange?.(page + 1)}><Icon name="chevron-right" /></button>
    </nav>
  );
}
