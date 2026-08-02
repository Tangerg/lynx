import React from 'react';

/** Circular identity chip with an image or initials fallback. */
export function Avatar({src, alt = '', name, size = 'md', shape = 'circle', className = '', children, ...rest}) {
  const initials = name ? name.trim().split(/\s+/).slice(0, 2).map(w => w[0]).join('').toUpperCase() : null;
  return (
    <span className={['avatar', size !== 'md' && `avatar--${size}`, shape === 'square' && 'avatar--square', className].filter(Boolean).join(' ')} {...rest}>
      {src ? <img className="avatar__image" src={src} alt={alt || name || ''} />
           : <span className="avatar__fallback">{children ?? initials}</span>}
    </span>
  );
}
Avatar.Group = function AvatarGroup({className = '', children, ...rest}) {
  return <span className={['avatar-group', className].filter(Boolean).join(' ')} {...rest}>{children}</span>;
};
