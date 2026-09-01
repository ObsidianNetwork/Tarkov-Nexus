import React, { InputHTMLAttributes, forwardRef, ReactNode } from 'react';
import { cn } from '../../utils';

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
  helperText?: ReactNode;
  leftIcon?: ReactNode;
  rightIcon?: ReactNode;
  wrapperClassName?: string;
}

export const Input = forwardRef<HTMLInputElement, InputProps>(
  (
    {
      className,
      label,
      error,
      helperText,
      leftIcon,
      rightIcon,
      wrapperClassName,
      required,
      disabled,
      ...props
    },
    ref
  ) => {
    return (
      <div className={cn('w-full', wrapperClassName)}>
        {label && (
          <label className="block text-sm font-medium text-text-secondary mb-2">
            {label}
            {required && <span className="text-error ml-1">*</span>}
          </label>
        )}

        <div className="relative">
          {leftIcon && (
            <div className="absolute left-3 top-1/2 -translate-y-1/2 text-text-muted flex-shrink-0">
              {leftIcon}
            </div>
          )}

          <input
            ref={ref}
            className={cn(
              'glass-input w-full px-4 py-2 transition-all duration-300',
              leftIcon && 'pl-10',
              rightIcon && 'pr-10',
              error && 'border-error ring-2 ring-error/20',
              disabled && 'opacity-50 cursor-not-allowed',
              className
            )}
            disabled={disabled}
            {...props}
          />

          {rightIcon && (
            <div className="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted flex-shrink-0">
              {rightIcon}
            </div>
          )}
        </div>

        {error && <p className="mt-1 text-sm text-error">{error}</p>}
        {helperText && !error && <p className="mt-1 text-sm text-text-muted">{helperText}</p>}
      </div>
    );
  }
);

Input.displayName = 'Input';

// Textarea variant
export interface TextareaProps extends React.TextareaHTMLAttributes<HTMLTextAreaElement> {
  label?: string;
  error?: string;
  helperText?: ReactNode;
  wrapperClassName?: string;
}

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, label, error, helperText, wrapperClassName, required, disabled, ...props }, ref) => {
    return (
      <div className={cn('w-full', wrapperClassName)}>
        {label && (
          <label className="block text-sm font-medium text-text-secondary mb-2">
            {label}
            {required && <span className="text-error ml-1">*</span>}
          </label>
        )}

        <textarea
          ref={ref}
          className={cn(
            'glass-input w-full px-4 py-2 transition-all duration-300 resize-none',
            error && 'border-error ring-2 ring-error/20',
            disabled && 'opacity-50 cursor-not-allowed',
            className
          )}
          disabled={disabled}
          {...props}
        />

        {error && <p className="mt-1 text-sm text-error">{error}</p>}
        {helperText && !error && <p className="mt-1 text-sm text-text-muted">{helperText}</p>}
      </div>
    );
  }
);

Textarea.displayName = 'Textarea';
