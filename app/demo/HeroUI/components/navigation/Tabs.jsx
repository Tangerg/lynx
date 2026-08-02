import React from 'react';

/** Tabs.ListContainer / List / Tab / Panel. */
export function Tabs({variant = 'segmented', selectedKey, defaultSelectedKey, onSelectionChange, orientation = 'horizontal', className = '', children, ...rest}) {
  const [inner, setInner] = React.useState(defaultSelectedKey);
  const current = selectedKey !== undefined ? selectedKey : inner;
  const select = k => { if (selectedKey === undefined) setInner(k); onSelectionChange?.(k); };
  const ctx = {current, select};
  return (
    <div className={['tabs', variant === 'underline' && 'tabs--underline', orientation === 'vertical' && 'tabs--vertical', className].filter(Boolean).join(' ')} {...rest}>
      {React.Children.map(children, c => React.isValidElement(c) ? React.cloneElement(c, {__tabs: ctx}) : c)}
    </div>
  );
}
Tabs.ListContainer = function TabsListContainer({__tabs, className = '', children, ...rest}) {
  return (
    <div className={['tabs__list-container', className].filter(Boolean).join(' ')} {...rest}>
      <div role="tablist" className="tabs__list">
        {React.Children.map(children, c => React.isValidElement(c) ? React.cloneElement(c, {__tabs}) : c)}
      </div>
    </div>
  );
};
Tabs.Tab = function Tab({id, __tabs, isDisabled, className = '', children, ...rest}) {
  const selected = __tabs?.current === id;
  return (
    <button type="button" role="tab" aria-selected={selected} disabled={isDisabled}
      data-selected={selected ? 'true' : undefined} data-disabled={isDisabled ? 'true' : undefined}
      className={['tabs__tab', className].filter(Boolean).join(' ')}
      onClick={() => !isDisabled && __tabs?.select(id)} {...rest}>{children}</button>
  );
};
Tabs.Panel = function TabsPanel({id, __tabs, className = '', children, ...rest}) {
  if (__tabs?.current !== id) return null;
  return <div role="tabpanel" className={['tabs__panel', className].filter(Boolean).join(' ')} {...rest}>{children}</div>;
};
