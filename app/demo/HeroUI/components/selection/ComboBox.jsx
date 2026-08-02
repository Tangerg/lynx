import React from 'react';
import {Icon} from '../content/Icon.jsx';
import {ListBox} from './ListBox.jsx';

/** Typeahead select. Filters `options` by the typed query. */
export function ComboBox({options = [], placeholder = 'Search…', onSelect, className = '', ...rest}) {
  const [query, setQuery] = React.useState('');
  const [open, setOpen] = React.useState(false);
  const matches = options.filter(o => String(o.label).toLowerCase().includes(query.toLowerCase()));
  return (
    <div style={{position: 'relative'}} className={className} {...rest}>
      <div className="input-group">
        <input className="input-group__input" value={query} placeholder={placeholder}
          onFocus={() => setOpen(true)} onChange={e => { setQuery(e.target.value); setOpen(true); }} />
        <span className="input-group__addon"><Icon name="chevron-down" /></span>
      </div>
      {open && matches.length > 0 && (
        <div style={{position: 'absolute', insetInlineStart: 0, insetInlineEnd: 0, marginTop: 6, zIndex: 40}}>
          <ListBox>
            {matches.map(o => (
              <ListBox.Item key={o.value} description={o.description}
                onClick={() => { setQuery(String(o.label)); setOpen(false); onSelect?.(o.value); }}>{o.label}</ListBox.Item>
            ))}
          </ListBox>
        </div>
      )}
    </div>
  );
}
