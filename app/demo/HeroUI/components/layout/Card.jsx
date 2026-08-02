import React from 'react';

/** Compound card. Card.Header / Title / Description / Content / Footer / Media. */
export function Card({variant = 'default', className = '', children, ...rest}) {
  return <div className={['card', variant !== 'default' && `card--${variant}`, className].filter(Boolean).join(' ')} {...rest}>{children}</div>;
}
Card.Header = function CardHeader({className = '', children, ...rest}) {
  return <div className={['card__header', className].filter(Boolean).join(' ')} {...rest}>{children}</div>;
};
Card.Title = function CardTitle({className = '', children, ...rest}) {
  return <h3 className={['card__title', className].filter(Boolean).join(' ')} {...rest}>{children}</h3>;
};
Card.Description = function CardDescription({className = '', children, ...rest}) {
  return <p className={['card__description', className].filter(Boolean).join(' ')} {...rest}>{children}</p>;
};
Card.Content = function CardContent({className = '', children, ...rest}) {
  return <div className={['card__content', className].filter(Boolean).join(' ')} {...rest}>{children}</div>;
};
Card.Footer = function CardFooter({className = '', children, ...rest}) {
  return <div className={['card__footer', className].filter(Boolean).join(' ')} {...rest}>{children}</div>;
};
Card.Media = function CardMedia({className = '', children, ...rest}) {
  return <figure className={['card__media', className].filter(Boolean).join(' ')} {...rest}>{children}</figure>;
};
