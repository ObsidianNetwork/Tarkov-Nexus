import React, { HTMLAttributes, forwardRef } from 'react';
import { cn } from '../../utils';

export interface CardProps extends HTMLAttributes<HTMLDivElement> {
  variant?: 'default' | 'featured';
  hoverable?: boolean;
  glowOnHover?: boolean;
}

export const Card = forwardRef<HTMLDivElement, CardProps>(
  ({ className, variant = 'default', hoverable = true, glowOnHover = false, children, ...props }, ref) => {
    const baseStyles = 'glass-card p-6 relative';
    const variantStyles = {
      default: '',
      featured: 'gradient-border',
    };
    const hoverStyles = hoverable ? 'hover-lift' : '';
    const glowStyles = glowOnHover ? 'hover-glow' : '';

    return (
      <div ref={ref} className={cn(baseStyles, variantStyles[variant], hoverStyles, glowStyles, className)} {...props}>
        {children}
      </div>
    );
  }
);

Card.displayName = 'Card';

// CardHeader component
export interface CardHeaderProps extends HTMLAttributes<HTMLDivElement> {
  title?: string;
  icon?: React.ReactNode;
  action?: React.ReactNode;
}

export const CardHeader = forwardRef<HTMLDivElement, CardHeaderProps>(
  ({ className, title, icon, action, children, ...props }, ref) => {
    return (
      <div ref={ref} className={cn('flex items-center justify-between mb-4', className)} {...props}>
        <div className="flex items-center gap-3">
          {icon && <div className="flex-shrink-0 text-primary-purple">{icon}</div>}
          {title && <h3 className="text-xl font-semibold text-text-primary">{title}</h3>}
          {children}
        </div>
        {action && <div className="flex-shrink-0">{action}</div>}
      </div>
    );
  }
);

CardHeader.displayName = 'CardHeader';

// CardContent component
export interface CardContentProps extends HTMLAttributes<HTMLDivElement> {}

export const CardContent = forwardRef<HTMLDivElement, CardContentProps>(
  ({ className, children, ...props }, ref) => {
    return (
      <div ref={ref} className={cn('text-text-secondary', className)} {...props}>
        {children}
      </div>
    );
  }
);

CardContent.displayName = 'CardContent';
