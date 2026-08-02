import React from 'react';

/** Button that stays on. Selected state inverts to foreground-on-background. */
export function ToggleButton({
  isSelected = false, size = 'md', isIconOnly = false, isDisabled = false,
  className = '', children, ...rest
}) {
  const cls = ['button', 'toggle-button', `button--${size}`,
    isIconOnly && 'button--icon-only', className].filter(Boolean).join(' ');
  return (
    <button type="button" className={cls} aria-pressed={isSelected} disabled={isDisabled}
      data-selected={isSelected ? 'true' : undefined} {...rest}>{children}</button>
  );
}
