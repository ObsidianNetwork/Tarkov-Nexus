import React, { ButtonHTMLAttributes, forwardRef, useState } from 'react';
import { cn } from '../../utils';

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger';
  size?: 'sm' | 'md' | 'lg';
  loading?: boolean;
  icon?: React.ReactNode;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'primary', size = 'md', loading, icon, children, disabled, onClick, ...props }, ref) => {
    const [ripples, setRipples] = useState<Array<{ x: number; y: number; id: number }>>([]);

    const handleClick = (e: React.MouseEvent<HTMLButtonElement>) => {
      if (disabled || loading) return;

      // Create ripple effect
      const button = e.currentTarget;
      const rect = button.getBoundingClientRect();
      const rippleSize = Math.max(rect.width, rect.height);
      const x = e.clientX - rect.left - rippleSize / 2;
      const y = e.clientY - rect.top - rippleSize / 2;
      const id = Date.now();

      setRipples((prev) => [...prev, { x, y, id }]);

      // Remove ripple after animation
      setTimeout(() => {
        setRipples((prev) => prev.filter((ripple) => ripple.id !== id));
      }, 600);

      onClick?.(e);
    };

    const baseStyles = 'relative overflow-hidden font-semibold transition-all duration-300 ease-out focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-offset-bg-dark disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2';

    const variantStyles = {
      primary:
        'bg-gradient-to-r from-primary-purple to-dark-purple text-white hover:shadow-glow-md hover:-translate-y-0.5 focus:ring-primary-purple',
      secondary:
        'bg-transparent border border-primary-purple text-primary-purple hover:bg-primary-purple/10 hover:-translate-y-0.5 hover:shadow-glow-sm focus:ring-primary-purple',
      ghost: 'bg-transparent text-text-secondary hover:bg-bg-hover hover:text-text-primary focus:ring-primary-purple',
      danger:
        'bg-gradient-to-r from-error to-red-600 text-white hover:shadow-[0_0_20px_rgba(237,66,69,0.4)] hover:-translate-y-0.5 focus:ring-error',
    };

    const sizeStyles = {
      sm: 'px-3 py-1.5 text-sm rounded-lg',
      md: 'px-4 py-2 text-base rounded-lg',
      lg: 'px-6 py-3 text-lg rounded-xl',
    };

    return (
      <button
        ref={ref}
        className={cn(baseStyles, variantStyles[variant], sizeStyles[size], className)}
        disabled={disabled || loading}
        onClick={handleClick}
        {...props}
      >
        {/* Ripple effects */}
        {ripples.map((ripple) => (
          <span
            key={ripple.id}
            className="absolute rounded-full bg-white/30 pointer-events-none animate-ripple"
            style={{
              left: ripple.x,
              top: ripple.y,
              width: Math.max(props.style?.width as number || 0, 100),
              height: Math.max(props.style?.height as number || 0, 100),
            }}
          />
        ))}

        {/* Loading spinner */}
        {loading && (
          <svg className="animate-spin h-5 w-5" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
            <circle
              className="opacity-25"
              cx="12"
              cy="12"
              r="10"
              stroke="currentColor"
              strokeWidth="4"
            ></circle>
            <path
              className="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            ></path>
          </svg>
        )}

        {/* Icon */}
        {icon && !loading && <span className="flex-shrink-0">{icon}</span>}

        {/* Content */}
        {children}
      </button>
    );
  }
);

Button.displayName = 'Button';
