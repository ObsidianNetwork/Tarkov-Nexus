import React, { HTMLAttributes, forwardRef } from 'react';
import { cn } from '../../utils';

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  variant?: 'success' | 'warning' | 'error' | 'info' | 'default';
  pulse?: boolean;
  glow?: boolean;
  icon?: React.ReactNode;
}

export const Badge = forwardRef<HTMLSpanElement, BadgeProps>(
  ({ className, variant = 'default', pulse = false, glow = false, icon, children, ...props }, ref) => {
    const baseStyles = 'inline-flex items-center gap-1.5 px-3 py-1 rounded-lg text-sm font-medium border transition-all duration-300';

    const variantStyles = {
      success: 'bg-neon-green/10 text-neon-green border-neon-green/20',
      warning: 'bg-warning/10 text-warning border-warning/20',
      error: 'bg-error/10 text-error border-error/20',
      info: 'bg-primary-purple/10 text-primary-purple border-primary-purple/20',
      default: 'bg-text-muted/10 text-text-secondary border-border-color/20',
    };

    const glowStyles = {
      success: glow ? 'shadow-glow-green' : '',
      warning: glow ? 'shadow-[0_0_20px_rgba(254,231,92,0.4)]' : '',
      error: glow ? 'shadow-[0_0_20px_rgba(237,66,69,0.4)]' : '',
      info: glow ? 'shadow-glow-md' : '',
      default: '',
    };

    return (
      <span ref={ref} className={cn(baseStyles, variantStyles[variant], glowStyles[variant], className)} {...props}>
        {pulse && (
          <span
            className={cn(
              'flex h-2 w-2 rounded-full animate-pulse',
              variant === 'success' && 'bg-neon-green',
              variant === 'warning' && 'bg-warning',
              variant === 'error' && 'bg-error',
              variant === 'info' && 'bg-primary-purple',
              variant === 'default' && 'bg-text-muted'
            )}
          />
        )}
        {icon && <span className="flex-shrink-0">{icon}</span>}
        {children}
      </span>
    );
  }
);

Badge.displayName = 'Badge';
