import React from 'react';

/** HeroUI v3 Button — pill (24px radius), 40px tall (36px ≥768px), press scales to 0.97. */
export function Button({
  variant = 'primary', size = 'md', isIconOnly = false, fullWidth = false,
  isDisabled = false, isPending = false, isPressed = false,
  startIcon, endIcon, className = '', children, ...rest
}) {
  const cls = ['button', `button--${variant}`, `button--${size}`,
    isIconOnly && 'button--icon-only', fullWidth && 'button--full-width', className]
    .filter(Boolean).join(' ');
  return (
    <button type="button" className={cls} disabled={isDisabled}
      aria-disabled={isDisabled || undefined}
      data-pending={isPending ? 'true' : undefined}
      data-pressed={isPressed ? 'true' : undefined} {...rest}>
      {isPending ? <span data-slot="spinner" className="spinner spinner--sm" /> : startIcon}
      {!isIconOnly && children}
      {isIconOnly && !isPending ? children : null}
      {endIcon}
    </button>
  );
}
