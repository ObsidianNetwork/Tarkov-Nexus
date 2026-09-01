import React, { HTMLAttributes, forwardRef } from 'react';
import { cn } from '../../utils';

export interface StatusIndicatorProps extends HTMLAttributes<HTMLDivElement> {
  status: 'success' | 'warning' | 'error' | 'offline';
  label?: string;
  pulse?: boolean;
  glow?: boolean;
  size?: 'sm' | 'md' | 'lg';
}

export const StatusIndicator = forwardRef<HTMLDivElement, StatusIndicatorProps>(
  ({ className, status, label, pulse = true, glow = true, size = 'md', ...props }, ref) => {
    const sizeStyles = {
      sm: 'h-2 w-2',
      md: 'h-3 w-3',
      lg: 'h-4 w-4',
    };

    const statusStyles = {
      success: 'bg-neon-green',
      warning: 'bg-warning',
      error: 'bg-error',
      offline: 'bg-text-muted',
    };

    const glowStyles = {
      success: 'shadow-glow-green',
      warning: 'shadow-[0_0_20px_rgba(254,231,92,0.5)]',
      error: 'shadow-[0_0_20px_rgba(237,66,69,0.5)]',
      offline: '',
    };

    return (
      <div ref={ref} className={cn('inline-flex items-center gap-2', className)} {...props}>
        <span className="relative flex">
          <span
            className={cn(
              'rounded-full',
              sizeStyles[size],
              statusStyles[status],
              pulse && 'animate-pulse',
              glow && glowStyles[status]
            )}
          />
          {pulse && (
            <span
              className={cn(
                'absolute inline-flex h-full w-full rounded-full opacity-75 animate-ping',
                statusStyles[status]
              )}
            />
          )}
        </span>
        {label && <span className="text-sm font-medium text-text-secondary">{label}</span>}
      </div>
    );
  }
);

StatusIndicator.displayName = 'StatusIndicator';
