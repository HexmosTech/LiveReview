import React, { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode, SelectHTMLAttributes, ElementType, ComponentPropsWithRef, useState, useRef, useLayoutEffect, useEffect, useMemo } from 'react';
import { createPortal } from 'react-dom';
import classNames from 'classnames';
import {
  SiGithub,
  SiGitlab,
  SiBitbucket,
  SiGitea,
  SiOllama,
  SiClaude,
  SiClaudecode,
  SiCursor,
  SiWindsurf,
  SiDeepseek,
  SiOpenrouter,
} from 'react-icons/si';
import { FaSlack, FaAws } from 'react-icons/fa6';
import { TbUserPlus, TbUsersPlus } from 'react-icons/tb';
import { VscVscode } from 'react-icons/vsc';

// ===== BUTTON COMPONENTS =====
type ButtonVariant = 'primary' | 'secondary' | 'outline' | 'ghost' | 'danger';
type ButtonSize = 'sm' | 'md' | 'lg';

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  icon?: ReactNode;
  iconPosition?: 'left' | 'right';
  isLoading?: boolean;
  fullWidth?: boolean;
  href?: string;
  as?: ElementType;
  to?: string; // For React Router Link
}

export const Button: React.FC<ButtonProps> = ({
  children,
  variant = 'primary',
  size = 'md',
  icon,
  iconPosition = 'left',
  isLoading = false,
  fullWidth = false,
  className,
  disabled,
  href,
  as,
  to,
  ...props
}) => {
  const baseStyles = 'inline-flex items-center justify-center font-medium rounded-lg transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2';
  
  const variantStyles = {
    primary: 'bg-blue-600 text-white hover:bg-blue-700 focus:ring-blue-500', // Primary Blue #3B82F6
    secondary: 'bg-slate-700 text-slate-100 hover:bg-slate-600 focus:ring-blue-400',
    outline: 'border border-slate-500 bg-transparent text-blue-300 hover:bg-slate-700 hover:text-blue-200 focus:ring-blue-400', // Dark Blue #1E40AF
    ghost: 'bg-transparent text-slate-300 hover:bg-slate-700 hover:text-slate-100 focus:ring-blue-400',
    danger: 'bg-red-600 text-white hover:bg-red-700 focus:ring-red-500',
  };
  
  const sizeStyles = {
    sm: 'text-xs px-3 py-2',
    md: 'text-sm px-4 py-2.5',
    lg: 'text-base px-5 py-3',
  };
  
  const loadingOrDisabled = isLoading || disabled;
  
  const buttonContent = (
    <>
      {isLoading && (
        <svg className="animate-spin -ml-1 mr-2 h-4 w-4 text-current" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
        </svg>
      )}
      {icon && iconPosition === 'left' && !isLoading && <span className="mr-2 w-5 flex items-center justify-center flex-shrink-0">{icon}</span>}
      {children}
      {icon && iconPosition === 'right' && <span className="ml-2">{icon}</span>}
    </>
  );
  
  // Common props for all element types
  const commonProps = {
    className: classNames(
      baseStyles,
      variantStyles[variant],
      sizeStyles[size],
      fullWidth ? 'w-full' : '',
      loadingOrDisabled ? 'opacity-70 cursor-not-allowed' : '',
      className
    ),
    disabled: loadingOrDisabled,
    ...props
  };
  
  // If a custom component is provided through 'as' prop (e.g., Link from react-router-dom)
  if (as) {
    const Component = as;
    return (
      <Component 
        {...commonProps}
        to={to} // Pass 'to' prop for react-router Link
      >
        {buttonContent}
      </Component>
    );
  }
  
  // If href is provided, render an anchor tag
  if (href) {
    return (
      <a
        href={href}
        className={classNames(
          baseStyles,
          variantStyles[variant],
          sizeStyles[size],
          fullWidth ? 'w-full' : '',
          loadingOrDisabled ? 'opacity-70 cursor-not-allowed' : '',
          className
        )}
      >
        {buttonContent}
      </a>
    );
  }
  
  // Otherwise render a button
  return (
    <button
      {...commonProps}
    >
      {buttonContent}
    </button>
  );
};

// ===== CARD COMPONENT =====
interface CardProps {
  children: ReactNode;
  className?: string;
  title?: ReactNode;
  subtitle?: ReactNode;
  footer?: ReactNode;
  badge?: string;
  badgeColor?: string;
}

export const Card: React.FC<CardProps> = ({
  children,
  className,
  title,
  subtitle,
  footer,
  badge,
  badgeColor = 'bg-blue-100 text-blue-800',
}) => {
  return (
    <div className={classNames(
      'bg-slate-800/90 rounded-lg shadow-sm border border-slate-700/80 backdrop-blur-sm', 
      'hover:shadow-md hover:border-slate-600/80 transition-all duration-200',
      className
    )}>
      {(title || subtitle || badge) && (
        <div className="flex justify-between items-center px-4 py-3 border-b border-slate-700/60">
          <div>
            {title && <h3 className="text-base font-semibold text-slate-100">{title}</h3>}
            {subtitle && <p className="text-sm text-slate-400 mt-0.5">{subtitle}</p>}
          </div>
          {badge && (
            <span className={`text-xs font-medium px-2 py-1 rounded-full ${badgeColor}`}>
              {badge}
            </span>
          )}
        </div>
      )}
      <div className="p-4">{children}</div>
      {footer && (
        <div className="border-t border-slate-700/60 px-4 py-3 bg-slate-900/50 rounded-b-lg">
          {footer}
        </div>
      )}
    </div>
  );
};

// ===== INPUT COMPONENTS =====
interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
  icon?: ReactNode;
  iconPosition?: 'left' | 'right';
  helperText?: string;
}

export const Input: React.FC<InputProps> = ({
  label,
  error,
  icon,
  iconPosition = 'left',
  helperText,
  className,
  id,
  ...props
}) => {
  const inputId = id || `input-${Math.random().toString(36).substr(2, 9)}`;
  
  return (
    <div className="w-full">
      {label && (
        <label htmlFor={inputId} className="block text-sm font-medium text-slate-300 mb-1">
          {label}
        </label>
      )}
      <div className="relative">
        {icon && iconPosition === 'left' && (
          <div className="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none text-slate-400">
            {icon}
          </div>
        )}
        <input
          id={inputId}
          className={classNames(
            'w-full rounded-lg border bg-slate-700 text-white focus:ring-2 transition-all duration-200 outline-none px-4 py-2.5',
            error 
              ? 'border-red-500 focus:border-red-500 focus:ring-red-400'
              : 'border-slate-600 focus:border-blue-500 focus:ring-blue-400',
            icon && iconPosition === 'left' ? 'pl-10' : '',
            icon && iconPosition === 'right' ? 'pr-10' : '',
            className
          )}
          aria-invalid={error ? 'true' : 'false'}
          autoComplete={props.autoComplete || 'off'}
          {...props}
        />
        {icon && iconPosition === 'right' && (
          <div className="absolute inset-y-0 right-0 pr-3 flex items-center pointer-events-none text-slate-400">
            {icon}
          </div>
        )}
      </div>
      {(error || helperText) && (
        <p className={classNames('mt-1 text-sm', error ? 'text-red-400' : 'text-slate-400')}>
          {error || helperText}
        </p>
      )}
    </div>
  );
};

// ===== SELECT COMPONENT =====
interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  label?: string;
  error?: string;
  helperText?: string;
  options: { value: string; label: string }[];
}

export const Select: React.FC<SelectProps> = ({
  label,
  error,
  helperText,
  options,
  className,
  id,
  ...props
}) => {
  const selectId = id || `select-${Math.random().toString(36).substr(2, 9)}`;
  
  return (
    <div className="w-full">
      {label && (
        <label htmlFor={selectId} className="block text-sm font-medium text-slate-300 mb-1">
          {label}
        </label>
      )}
      <div className="relative">
        <select
          id={selectId}
          className={classNames(
            'w-full rounded-lg border focus:ring-2 transition-all duration-200 outline-none px-4 py-2.5 appearance-none bg-slate-700 text-white',
            error 
              ? 'border-red-500 focus:border-red-500 focus:ring-red-400' 
              : 'border-slate-600 focus:border-blue-500 focus:ring-blue-400',
            className
          )}
          aria-invalid={error ? 'true' : 'false'}
          {...props}
        >
          {options.map((option) => (
            <option key={option.value} value={option.value} className="bg-slate-700">
              {option.label}
            </option>
          ))}
        </select>
        <div className="pointer-events-none absolute inset-y-0 right-0 flex items-center px-2 text-slate-300">
          <svg className="h-5 w-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clipRule="evenodd" />
          </svg>
        </div>
      </div>
      {(error || helperText) && (
        <p className={classNames('mt-1 text-sm', error ? 'text-red-400' : 'text-slate-400')}>
          {error || helperText}
        </p>
      )}
    </div>
  );
};

// ===== BADGE COMPONENT =====
type BadgeVariant = 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'info';
type BadgeSize = 'sm' | 'md';

interface BadgeProps {
  children: ReactNode;
  variant?: BadgeVariant;
  size?: BadgeSize;
  className?: string;
}

export const Badge: React.FC<BadgeProps> = ({
  children,
  variant = 'default',
  size = 'md',
  className,
}) => {
  const variantStyles = {
    default: 'bg-gray-100 text-gray-800',
    primary: 'bg-blue-100 text-blue-800',
    success: 'bg-green-100 text-green-800',
    warning: 'bg-yellow-100 text-yellow-800',
    danger: 'bg-red-100 text-red-800',
    info: 'bg-blue-100 text-blue-800',
  };
  
  const sizeStyles = {
    sm: 'text-xs px-2 py-0.5',
    md: 'text-sm px-2.5 py-1',
  };
  
  return (
    <span 
      className={classNames(
        'font-medium rounded-full inline-flex items-center',
        variantStyles[variant],
        sizeStyles[size],
        className
      )}
    >
      {children}
    </span>
  );
};

// ===== TOGGLE COMPONENT =====
interface ToggleProps {
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
  isLoading?: boolean;
  className?: string;
  'aria-label'?: string;
}

export const Toggle: React.FC<ToggleProps> = ({
  checked,
  onChange,
  disabled = false,
  isLoading = false,
  className,
  'aria-label': ariaLabel,
}) => {
  const inactive = disabled || isLoading;
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={ariaLabel}
      disabled={inactive}
      onClick={() => !inactive && onChange(!checked)}
      className={classNames(
        'relative inline-flex h-6 w-11 flex-shrink-0 items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-blue-400 focus:ring-offset-2 focus:ring-offset-slate-900',
        checked ? 'bg-blue-600' : 'bg-slate-600',
        inactive ? 'opacity-60 cursor-not-allowed' : 'cursor-pointer',
        className
      )}
    >
      <span
        className={classNames(
          'inline-block h-4 w-4 transform rounded-full bg-white transition-transform',
          checked ? 'translate-x-6' : 'translate-x-1'
        )}
      />
    </button>
  );
};

// ===== EMPTY STATE COMPONENT =====
interface EmptyStateProps {
  icon?: ReactNode;
  title: string;
  description?: string;
  action?: ReactNode;
  className?: string;
}

export const EmptyState: React.FC<EmptyStateProps> = ({
  icon,
  title,
  description,
  action,
  className,
}) => {
  return (
    <div className={classNames('flex flex-col items-center justify-center py-10 text-center', className)}>
      {icon && <div className="mb-4 text-slate-400">{icon}</div>}
      <h3 className="text-lg font-medium text-white mb-1">{title}</h3>
      {description && <p className="text-sm text-slate-300 max-w-sm mb-4">{description}</p>}
      {action && <div>{action}</div>}
    </div>
  );
};

// ===== AVATAR COMPONENT =====
type AvatarSize = 'xs' | 'sm' | 'md' | 'lg' | 'xl';

interface AvatarProps {
  src?: string;
  alt?: string;
  initials?: string;
  size?: AvatarSize;
  className?: string;
}

export const Avatar: React.FC<AvatarProps> = ({
  src,
  alt = 'Avatar',
  initials,
  size = 'md',
  className,
}) => {
  const sizeStyles = {
    xs: 'w-6 h-6 text-xs',
    sm: 'w-8 h-8 text-sm',
    md: 'w-10 h-10 text-base',
    lg: 'w-12 h-12 text-lg',
    xl: 'w-16 h-16 text-xl',
  };
  
  if (src) {
    return (
      <img
        src={src}
        alt={alt}
        className={classNames('rounded-full object-cover', sizeStyles[size], className)}
      />
    );
  }
  
  return (
    <div className={classNames(
      'rounded-full flex items-center justify-center bg-indigo-100 text-indigo-800 font-medium',
      sizeStyles[size],
      className
    )}>
      {initials}
    </div>
  );
};

// ===== STAT CARD COMPONENT =====
interface StatCardProps {
  title: string;
  value: string | number;
  icon?: ReactNode;
  trend?: {
    value: number;
    label?: string;
    isPositive?: boolean;
  };
  className?: string;
  variant?: 'default' | 'primary';
  description?: string;
  // Optional mini-CTA shown when value is 0
  emptyCtaLabel?: string;
  onEmptyCta?: () => void;
  emptyNote?: string;
  // Optional tooltip content to show an Info icon in the header
  headerInfo?: string;
  headerInfoPosition?: 'top' | 'bottom' | 'left' | 'right' | 'top-left' | 'top-right' | 'bottom-left' | 'bottom-right' | 'auto';
}

export const StatCard: React.FC<StatCardProps> = ({
  title,
  value,
  icon,
  trend,
  className,
  variant = 'default',
  description,
  emptyCtaLabel,
  onEmptyCta,
  emptyNote,
  headerInfo,
  headerInfoPosition = 'auto',
}) => {
  const isZero = Number(value) === 0;
  if (variant === 'primary') {
    return (
      <Card className={classNames('flex flex-col border-l-4 border-l-blue-500 bg-slate-800/80 hover:bg-slate-800 transition-colors', className)}>
        <div className="flex items-center justify-between">
          <div className="flex-grow">
            <p className="text-sm font-medium text-blue-300 mb-1">{title}</p>
            <div className="flex items-baseline">
              <p className="text-2xl font-bold text-white">{value}</p>
              {trend && (
                <div className={classNames(
                  'ml-2 flex items-center text-xs font-medium',
                  trend.isPositive ? 'text-green-400' : 'text-red-400'
                )}>
                  <span>{trend.isPositive ? '↑' : '↓'} {Math.abs(trend.value)}%</span>
                  {trend.label && <span className="text-slate-300 ml-1">{trend.label}</span>}
                </div>
              )}
            </div>
            {description && <p className="mt-1 text-xs text-slate-400">{description}</p>}
            {isZero && (emptyCtaLabel || emptyNote) && (
              <div className="mt-2 text-xs text-slate-400 flex items-center gap-2">
                {emptyNote && <span>{emptyNote}</span>}
                {onEmptyCta && (
                  <button
                    type="button"
                    onClick={onEmptyCta}
                    className="text-blue-300 hover:text-blue-200 underline"
                  >
                    {emptyCtaLabel || 'Learn more'}
                  </button>
                )}
              </div>
            )}
          </div>
          {(icon || headerInfo) && (
            <div className="ml-3 flex items-center gap-2">
              {headerInfo && (
                <Tooltip content={headerInfo} position={headerInfoPosition}>
                  <span className="inline-flex items-center text-slate-300"><Icons.Info /></span>
                </Tooltip>
              )}
              {icon && (
                <div className="p-2 bg-blue-600/20 rounded-lg">
                  <div className="text-blue-400 text-lg">{icon}</div>
                </div>
              )}
            </div>
          )}
        </div>
      </Card>
    );
  }
  
  return (
    <Card className={classNames('flex flex-col bg-slate-800/60 hover:bg-slate-800/80 transition-colors', className)}>
      <div className="flex items-center justify-between mb-2">
        <p className="text-sm font-medium text-slate-300">{title}</p>
        {(icon || headerInfo) && (
          <div className="flex items-center gap-2">
            {headerInfo && (
              <Tooltip content={headerInfo} position={headerInfoPosition}>
                <span className="inline-flex items-center text-slate-400"><Icons.Info /></span>
              </Tooltip>
            )}
            {icon && <div className="text-slate-400 text-sm">{icon}</div>}
          </div>
        )}
      </div>
      <div className="flex items-baseline">
        <p className="text-xl font-bold text-white">{value}</p>
        {trend && (
          <div className={classNames(
            'ml-2 flex items-center text-xs font-medium',
            trend.isPositive ? 'text-green-400' : 'text-red-400'
          )}>
            <span>{trend.isPositive ? '↑' : '↓'} {Math.abs(trend.value)}%</span>
            {trend.label && <span className="text-slate-300 ml-1">{trend.label}</span>}
          </div>
        )}
      </div>
      {description && <p className="mt-1 text-xs text-slate-400">{description}</p>}
      {isZero && (emptyCtaLabel || emptyNote) && (
        <div className="mt-2 text-xs text-slate-400 flex items-center gap-2">
          {emptyNote && <span>{emptyNote}</span>}
          {onEmptyCta && (
            <button
              type="button"
              onClick={onEmptyCta}
              className="text-blue-300 hover:text-blue-200 underline"
            >
              {emptyCtaLabel || 'Learn more'}
            </button>
          )}
        </div>
      )}
    </Card>
  );
};

// ===== ALERT COMPONENT =====
type AlertVariant = 'info' | 'success' | 'warning' | 'error';

interface AlertProps {
  title?: string;
  children: ReactNode;
  variant?: AlertVariant;
  icon?: ReactNode;
  onClose?: () => void;
  className?: string;
  floating?: boolean;
}

export const Alert: React.FC<AlertProps> = ({
  title,
  children,
  variant = 'info',
  icon,
  onClose,
  className,
  floating,
}) => {
  const variantStyles = {
    info: 'bg-blue-900/80 text-blue-100 border-blue-700/50',
    success: 'bg-green-900/80 text-green-100 border-green-700/50',
    warning: 'bg-amber-900/80 text-amber-100 border-amber-700/50',
    error: 'bg-red-900/80 text-red-100 border-red-700/50',
  };

  return (
    <div className={classNames(
      'rounded-lg border p-4',
      variantStyles[variant],
      floating && 'absolute top-0 left-0 right-0 z-10 shadow-lg shadow-black/20',
      className
    )}>
      <div className="flex">
        {icon && <div className="flex-shrink-0 mr-3">{icon}</div>}
        <div className="flex-1">
          {title && <h3 className="text-sm font-medium mb-1">{title}</h3>}
          <div className="text-sm">{children}</div>
        </div>
        {onClose && (
          <button
            type="button"
            className="ml-auto -mx-1.5 -my-1.5 rounded-lg p-1.5 inline-flex focus:outline-none focus:ring-2 focus:ring-offset-2"
            onClick={onClose}
          >
            <span className="sr-only">Dismiss</span>
            <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
            </svg>
          </button>
        )}
      </div>
    </div>
  );
};

// Common SVG icons to use throughout the app
export const Icons = {
  // Navigation icons
  Dashboard: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zm10 0a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zm10 0a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z" />
    </svg>
  ),
  Git: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" />
    </svg>
  ),
  AI: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z" />
    </svg>
  ),
  Reviews: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
    </svg>
  ),
  Settings: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
    </svg>
  ),
  Reports: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
    </svg>
  ),
  
  // Action icons
  Add: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
    </svg>
  ),
  UserAdd: () => <TbUserPlus className="w-5 h-5" />,
  GroupAdd: () => <TbUsersPlus className="w-5 h-5" />,
  Edit: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
    </svg>
  ),
  Delete: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
    </svg>
  ),
  Close: () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
    </svg>
  ),
  
  // State icons
  EmptyState: () => (
    <svg className="w-16 h-16 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1} d="M17 14v6m-3-3h6M6 10h2a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v2a2 2 0 002 2zm10 0h2a2 2 0 002-2V6a2 2 0 00-2-2h-2a2 2 0 00-2 2v2a2 2 0 002 2zM6 20h2a2 2 0 002-2v-2a2 2 0 00-2-2H6a2 2 0 00-2 2v2a2 2 0 002 2z" />
    </svg>
  ),
  Info: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
    </svg>
  ),
  Success: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
    </svg>
  ),
  Warning: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
    </svg>
  ),
  Bell: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
    </svg>
  ),
  Error: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
    </svg>
  ),

  // Logo icons (from react-icons — Simple Icons / Font Awesome brand sets), tinted with each
  // brand's official color (per the simple-icons package data). GitHub/OpenAI/Ollama publish
  // near-black as their official color, which is invisible on this app's dark-only UI, so those
  // three use the brands' official white-on-dark variant instead.
  GitHub: () => <SiGithub className="w-5 h-5 text-white" />,
  GitLab: () => <SiGitlab className="w-5 h-5 text-[#FC6D26]" />,
  Bitbucket: () => <SiBitbucket className="w-5 h-5 text-[#0052CC]" />,
  Gitea: () => <SiGitea className="w-5 h-5 text-[#609926]" />,
  Claude: () => <SiClaude className="w-5 h-5 text-[#D97757]" />,
  // Anthropic Compatible (custom Claude-API-shaped endpoints) — supplied SVG, inlined so
  // text-[hex] can tint it, distinct from the Claude mark used for the main Anthropic provider.
  AnthropicCompatible: () => (
    <svg className="w-5 h-5 text-[#D97757]" fill="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path d="M7.47 5.13 2 18.86h3.06l1.12-2.88h5.73l1.12 2.88h3.06L10.62 5.13H7.48Zm3.44 8.3H7.16L9.03 8.6l1.87 4.83ZM13.52 5.13 19 18.87h3L16.53 5.13z" />
    </svg>
  ),
  // Claude Code CLI mark (distinct Simple Icons entry from the general Anthropic/Claude mark above).
  ClaudeCode: () => <SiClaudecode className="w-5 h-5 text-[#D97757]" />,
  // Cursor/Windsurf publish near-black/black as their official color — invisible on this app's
  // dark-only UI, so (matching GitHub/OpenAI/Ollama above) they use white instead.
  Cursor: () => <SiCursor className="w-5 h-5 text-white" />,
  Windsurf: () => <SiWindsurf className="w-5 h-5 text-white" />,
  // VS Code's own icon package (not Simple Icons) — real brand mark, official blue.
  VSCode: () => <VscVscode className="w-5 h-5 text-[#007ACC]" />,
  DeepSeek: () => <SiDeepseek className="w-5 h-5 text-[#5786FE]" />,
  OpenRouter: () => <SiOpenrouter className="w-5 h-5 text-[#94A3B8]" />,
  Slack: ({ className = 'w-5 h-5 text-[#4A154B]' }: { className?: string } = {}) => <FaSlack className={className} />,
  Aws: () => <FaAws className="w-5 h-5 text-[#FF9900]" />,
  // Not on Simple Icons / Font Awesome (removed or never added for these brands) — kept hand-drawn.
  AzureDevOps: () => (
    <svg className="w-5 h-5 text-[#0078D7]" fill="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path d="M23.995 6.617l-4.55-3.556-9.617 3.605V2.44L3.36 8.404 0 9.877v4.396l3.36-1.377v4.75l6.467 2.914 10.107-4.895V6.617zM9.828 17.535l-4.83-1.976v-3.605l4.83 2.183v3.398zm.687-9.096l3.925 2.79-6.29 2.516-3.176-1.436 5.541-3.87zm10.083 8.13l-8.207 3.95v-3.328l5.28-2.176V8.988l2.927 2.293v5.288z"/>
    </svg>
  ),
  OpenAI: () => (
    <svg className="w-5 h-5 text-white" fill="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path d="M22.2819 9.8211a5.9847 5.9847 0 0 0-.5157-4.9108 6.0462 6.0462 0 0 0-6.5098-2.9A6.0651 6.0651 0 0 0 4.9807 4.1818a5.9847 5.9847 0 0 0-3.9977 2.9 6.0462 6.0462 0 0 0 .7427 7.0966 5.98 5.98 0 0 0 .511 4.9107 6.051 6.051 0 0 0 6.5146 2.9001A5.9847 5.9847 0 0 0 13.2599 24a6.0557 6.0557 0 0 0 5.7718-4.2058 5.9894 5.9894 0 0 0 3.9977-2.9001 6.0557 6.0557 0 0 0-.7475-7.0729zm-9.022 12.6081a4.4755 4.4755 0 0 1-2.8764-1.0408l.1419-.0804 4.7783-2.7582a.7948.7948 0 0 0 .3927-.6813v-6.7369l2.02 1.1686a.071.071 0 0 1 .038.052v5.5826a4.504 4.504 0 0 1-4.4945 4.4944zm-9.6607-4.1254a4.4708 4.4708 0 0 1-.5346-3.0137l.142.0852 4.783 2.7582a.7712.7712 0 0 0 .7806 0l5.8428-3.3685v2.3324a.0804.0804 0 0 1-.0332.0615L9.74 19.9502a4.4992 4.4992 0 0 1-6.1408-1.6464zM2.3408 7.8956a4.485 4.485 0 0 1 2.3655-1.9728V11.6a.7664.7664 0 0 0 .3879.6765l5.8144 3.3543-2.0201 1.1685a.0757.0757 0 0 1-.071 0l-4.8303-2.7865A4.504 4.504 0 0 1 2.3408 7.872zm16.5963 3.8558L13.1038 8.364 15.1192 7.2a.0757.0757 0 0 1 .071 0l4.8303 2.7913a4.4944 4.4944 0 0 1-.6765 8.1042v-5.6772a.79.79 0 0 0-.407-.667zm2.0107-3.0231l-.142-.0852-4.7735-2.7818a.7759.7759 0 0 0-.7854 0L9.409 9.2297V6.8974a.0662.0662 0 0 1 .0284-.0615l4.8303-2.7866a4.4992 4.4992 0 0 1 6.6802 4.66zM8.3065 12.863l-2.02-1.1638a.0804.0804 0 0 1-.038-.0567V6.0742a4.4992 4.4992 0 0 1 7.3757-3.4537l-.142.0805L8.704 5.459a.7948.7948 0 0 0-.3927.6813zm1.0976-2.3654l2.602-1.4998 2.6069 1.4998v2.9994l-2.5974 1.5093-2.6067-1.4998z"/>
    </svg>
  ),
  // Used only for Google Gemini AI provider entries — the Gemini mark, not the generic Google "G".
  // Official full-color mark supplied as SVG; loaded as an image since its gradients/filters
  // aren't tintable via currentColor.
  Google: ({ className = 'w-5 h-5' }: { className?: string } = {}) => (
    <img src="/assets/gemini.svg" alt="" className={className} />
  ),
  Ollama: () => <SiOllama className="w-5 h-5 text-white" />,
  // Official full-color mark supplied as SVG; loaded as an image since its multi-tone fills
  // aren't tintable via currentColor.
  Cohere: ({ className = 'w-5 h-5' }: { className?: string } = {}) => (
    <img src="/assets/Cohere.svg" alt="" className={className} />
  ),
  // Official mark supplied as a raster PNG (no SVG available) — loaded as an image.
  AtlasCloud: ({ className = 'w-5 h-5 rounded-full' }: { className?: string } = {}) => (
    <img src="/assets/atlascloud.png" alt="" className={className} />
  ),
  // Microsoft Teams isn't in Simple Icons or Font Awesome's brand set — kept as the official logo asset.
  Teams: ({ className = 'w-5 h-5 rounded-sm' }: { className?: string } = {}) => (
    <img src="/assets/teams-logo.svg" alt="" className={className} />
  ),
  Discord: ({ className = 'w-5 h-5 rounded-sm' }: { className?: string } = {}) => (
    <img src="/assets/discord-logo.svg" alt="" className={className} />
  ),
  
  // Chat icon
  Chat: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
    </svg>
  ),

  // Contact icons
  Email: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
    </svg>
  ),
  User: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
    </svg>
  ),
  
  // Status icons
  Clock: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
    </svg>
  ),
  Search: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="m21 21-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
    </svg>
  ),
  Filter: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z" />
    </svg>
  ),
  NotReady: () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
    </svg>
  ),
  Ready: () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
    </svg>
  ),
  
  // Tree view icons
  ChevronRight: () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="m9 18 6-6-6-6" />
    </svg>
  ),
  ChevronDown: () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="m6 9 6 6 6-6" />
    </svg>
  ),
  Copy: () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
    </svg>
  ),
  Folder: () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-5l-2-2H5a2 2 0 00-2 2z" />
    </svg>
  ),
  FolderOpen: () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 19a2 2 0 01-2-2V7a2 2 0 012-2h4l2 2h4a2 2 0 012 2v1M5 19h14a2 2 0 002-2v-5a2 2 0 00-2-2H9a2 2 0 00-2 2v5a2 2 0 01-2 2z" />
    </svg>
  ),
  
  // View toggle icons
  List: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 10h16M4 14h16M4 18h16" />
    </svg>
  ),
  Grid: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
    </svg>
  ),
  Layers: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 2 3 7l9 5 9-5-9-5zM3 12l9 5 9-5M3 17l9 5 9-5" />
    </svg>
  ),
  Refresh: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
    </svg>
  ),
  Download: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v2a2 2 0 002 2h12a2 2 0 002-2v-2M7 10l5 5 5-5M12 15V3" />
    </svg>
  ),
};

// ===== LAYOUT COMPONENTS =====
interface PageHeaderProps {
  title: string;
  description?: string;
  actions?: ReactNode;
  className?: string;
}

export const PageHeader: React.FC<PageHeaderProps> = ({
  title,
  description,
  actions,
  className,
}) => {
  return (
    <div className={classNames('mb-8', className)}>
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">{title}</h1>
          {description && <p className="mt-2 text-base text-slate-300">{description}</p>}
        </div>
        {actions && <div>{actions}</div>}
      </div>
    </div>
  );
};

interface SectionProps {
  title?: string;
  description?: string;
  children: ReactNode;
  className?: string;
}

export const Section: React.FC<SectionProps> = ({
  title,
  description,
  children,
  className,
}) => {
  return (
    <div className={classNames('mb-8', className)}>
      {(title || description) && (
        <div className="mb-4">
          {title && <h2 className="text-lg font-medium text-white">{title}</h2>}
          {description && <p className="mt-1 text-sm text-slate-300">{description}</p>}
        </div>
      )}
      {children}
    </div>
  );
};

// ===== CONTAINER LAYOUT =====
interface ContainerProps {
  children: ReactNode;
  className?: string;
  maxWidth?: 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl' | 'full';
}

export const Container: React.FC<ContainerProps> = ({
  children,
  className,
  maxWidth = 'xl',
}) => {
  const maxWidthClasses = {
    xs: 'max-w-xs',
    sm: 'max-w-sm',
    md: 'max-w-md',
    lg: 'max-w-lg',
    xl: 'max-w-xl',
    '2xl': 'max-w-2xl',
    full: 'max-w-full',
  };
  
  return (
    <div className={classNames(
      'mx-auto w-full px-4 sm:px-6', 
      maxWidthClasses[maxWidth],
      className
    )}>
      {children}
    </div>
  );
};

// ===== SIMPLE POPOVER COMPONENT (moved below to avoid interfering with other declarations) =====
interface PopoverProps {
  trigger: ReactNode;
  children: ReactNode;
  align?: 'left' | 'right' | 'center';
  className?: string;
  hover?: boolean; // open on hover instead of click
  delay?: number; // closing delay for hover
  // Used only for viewport clamp/flip math below, since that has to happen
  // before the (portal-rendered) content has a real measured size — mirrors
  // git-lrc's RiskBadge.js CARD_WIDTH/CARD_EST_HEIGHT constants.
  estimatedWidth?: number;
  estimatedHeight?: number;
}

export const Popover: React.FC<PopoverProps> = ({
  trigger, children, align = 'left', className, hover = false, delay = 180,
  estimatedWidth = 320, estimatedHeight = 280,
}) => {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLDivElement | null>(null);
  const popoverRef = useRef<HTMLDivElement | null>(null);
  const closeTimer = useRef<number | null>(null);
  const [position, setPosition] = useState<{ top: number; left: number; flipped: boolean }>({ top: 0, left: 0, flipped: false });

  const computePosition = () => {
    if (!triggerRef.current) return;
    const rect = triggerRef.current.getBoundingClientRect();
    let left = rect.left;
    if (align === 'center') left = rect.left + rect.width / 2 - estimatedWidth / 2;
    if (align === 'right') left = rect.right - estimatedWidth;
    // Clamp horizontally so the card never runs off either viewport edge.
    left = Math.max(8, Math.min(left, window.innerWidth - estimatedWidth - 12));

    // Flip above the trigger when there isn't room below but there is above —
    // otherwise a badge deep in a long page renders its card off-screen.
    const flipped = rect.bottom + estimatedHeight > window.innerHeight && rect.top > estimatedHeight;
    const top = flipped ? rect.top + window.scrollY - estimatedHeight - 6 : rect.bottom + window.scrollY + 6;

    setPosition({ top, left: left + window.scrollX, flipped });
  };

  useLayoutEffect(() => { if (open) computePosition(); }, [open, align]);
  React.useEffect(() => { const onResize = () => open && computePosition(); window.addEventListener('resize', onResize); return () => window.removeEventListener('resize', onResize); }, [open]);

  React.useEffect(() => {
    if (hover) return; // click mode listeners only
    const onClickOutside = (e: MouseEvent) => {
      if (!open) return;
      if (popoverRef.current && !popoverRef.current.contains(e.target as Node) && triggerRef.current && !triggerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    const onEsc = (e: KeyboardEvent) => { if (e.key === 'Escape') setOpen(false); };
    document.addEventListener('mousedown', onClickOutside);
    document.addEventListener('keydown', onEsc);
    return () => { document.removeEventListener('mousedown', onClickOutside); document.removeEventListener('keydown', onEsc); };
  }, [open, hover]);

  const clearTimer = () => { if (closeTimer.current) { window.clearTimeout(closeTimer.current); closeTimer.current = null; } };
  const scheduleClose = () => { clearTimer(); closeTimer.current = window.setTimeout(() => setOpen(false), delay); };

  const triggerProps = hover ? {
    onMouseEnter: () => { clearTimer(); setOpen(true); },
    onMouseLeave: () => scheduleClose(),
    onFocus: () => setOpen(true),
    onBlur: () => scheduleClose(),
  } : {
    onClick: () => setOpen(o => !o)
  };

  const content = open ? (
    <div
      ref={popoverRef}
      className={classNames(
        'z-50 absolute w-80 rounded-md shadow-lg border border-slate-600 bg-slate-800 text-slate-200 text-sm p-4 animate-fadeIn',
        className
      )}
      style={{ top: position.top, left: position.left }}
      role="dialog"
      aria-modal="false"
      onMouseEnter={hover ? clearTimer : undefined}
      onMouseLeave={hover ? scheduleClose : undefined}
    >
      {children}
    </div>
  ) : null;

  return (
    <>
      <div ref={triggerRef} className="inline-flex" {...triggerProps}>
        {trigger}
      </div>
      {createPortal(content, document.body)}
    </>
  );
};

// ===== SPINNER COMPONENT =====
interface SpinnerProps {
  size?: 'sm' | 'md' | 'lg';
  color?: string;
  className?: string;
}

export const Spinner: React.FC<SpinnerProps> = ({
  size = 'md',
  color = 'text-blue-500',
  className,
}) => {
  const sizeClasses = {
    sm: 'w-4 h-4',
    md: 'w-8 h-8',
    lg: 'w-12 h-12',
  };
  
  return (
    <div className={classNames('inline-block', className)}>
      <svg 
        className={classNames('animate-spin', sizeClasses[size], color)} 
        xmlns="http://www.w3.org/2000/svg" 
        fill="none" 
        viewBox="0 0 24 24"
      >
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
    </div>
  );
};

// ===== TOOLTIP COMPONENT =====
interface TooltipProps {
  children: ReactNode;
  content: ReactNode;
  position?: 'top' | 'bottom' | 'left' | 'right' | 'top-left' | 'top-right' | 'bottom-left' | 'bottom-right' | 'auto';
  className?: string;
}

export const Tooltip: React.FC<TooltipProps> = ({
  children,
  content,
  position = 'auto',
  className,
}) => {
  const [isVisible, setIsVisible] = useState(false);
  const [resolvedPosition, setResolvedPosition] = useState<TooltipProps['position']>('top');
  const [coords, setCoords] = useState<{top:number; left:number; arrowLeft?: number}>({ top: 0, left: 0, arrowLeft: undefined });
  const wrapperRef = useRef<HTMLDivElement>(null);
  const tipRef = useRef<HTMLDivElement>(null);

  const clamp = (val: number, min: number, max: number) => Math.max(min, Math.min(max, val));

  const choosePlacement = () => {
    if (!wrapperRef.current) return 'top' as TooltipProps['position'];
    if (position && position !== 'auto') return position;
    const rect = wrapperRef.current.getBoundingClientRect();
    const edge = 12;
    const preferTop = rect.top > window.innerHeight * 0.33; // if not near very top
    // try to keep inside horizontally too
    if (preferTop) return 'top';
    return 'bottom';
  };

  const positionTooltip = (placement: TooltipProps['position']) => {
    if (!wrapperRef.current) return;
    const rect = wrapperRef.current.getBoundingClientRect();
    const estWidth = tipRef.current?.offsetWidth || 240;
    const estHeight = tipRef.current?.offsetHeight || 40;
    const margin = 8;
    let top = 0;
    let left = 0;
    let arrowLeft: number | undefined;
    const centerX = rect.left + rect.width / 2;
    if (placement.startsWith('bottom')) {
      top = rect.bottom + margin;
    } else if (placement.startsWith('top')) {
      top = rect.top - estHeight - margin;
    } else if (placement === 'left') {
      top = rect.top + rect.height / 2 - estHeight / 2;
      left = rect.left - estWidth - margin;
    } else if (placement === 'right') {
      top = rect.top + rect.height / 2 - estHeight / 2;
      left = rect.right + margin;
    }
    if (placement.endsWith('left')) {
      left = rect.left;
      arrowLeft = clamp((centerX - left), 8, estWidth - 8);
    } else if (placement.endsWith('right')) {
      left = rect.right - estWidth;
      arrowLeft = clamp((centerX - left), 8, estWidth - 8);
    } else if (placement === 'top' || placement === 'bottom') {
      left = centerX - estWidth / 2;
      arrowLeft = estWidth / 2;
    }
    // viewport clamp
    const clampedLeft = clamp(left, 8, window.innerWidth - estWidth - 8);
    // adjust arrow when we clamp horizontally
    if (arrowLeft !== undefined) {
      arrowLeft = clamp(arrowLeft + (left - clampedLeft), 8, estWidth - 8);
    }
    setCoords({ top: Math.max(8, top), left: clampedLeft, arrowLeft });
  };

  const onEnter = () => {
    const p = choosePlacement();
    setResolvedPosition(p);
    setIsVisible(true);
  };
  const onLeave = () => setIsVisible(false);

  useLayoutEffect(() => {
    if (!isVisible) return;
    positionTooltip(resolvedPosition);
  }, [isVisible, resolvedPosition, content]);

  return (
    <div ref={wrapperRef} className="relative inline-block" onMouseEnter={onEnter} onMouseLeave={onLeave}>
      {children}
      {isVisible && createPortal(
        <div
          ref={tipRef}
          className={classNames(
            'fixed z-[9999] px-2 py-1 text-xs font-medium text-white bg-gray-900 rounded shadow-lg whitespace-normal break-words w-max max-w-[min(80vw,24rem)] pointer-events-none',
            className
          )}
          style={{ top: coords.top, left: coords.left }}
        >
          {content}
          {/* arrow */}
          <div
            className="absolute w-0 h-0"
            style={{
              left: coords.arrowLeft !== undefined ? coords.arrowLeft : undefined,
              right: coords.arrowLeft === undefined ? 8 : undefined,
              top: resolvedPosition.startsWith('bottom') ? -6 : undefined,
              bottom: resolvedPosition.startsWith('top') ? -6 : undefined,
              borderLeft: '6px solid transparent',
              borderRight: '6px solid transparent',
              borderTop: resolvedPosition.startsWith('bottom') ? '6px solid #111827' : undefined,
              borderBottom: resolvedPosition.startsWith('top') ? '6px solid #111827' : undefined,
            }}
          />
        </div>,
        document.body
      )}
    </div>
  );
};

// ===== DIVIDER COMPONENT =====
interface DividerProps {
  label?: string;
  className?: string;
}

export const Divider: React.FC<DividerProps> = ({ label, className }) => {
  if (label) {
    return (
      <div className={classNames('relative', className)}>
        <div className="absolute inset-0 flex items-center">
          <div className="w-full border-t border-slate-600"></div>
        </div>
        <div className="relative flex justify-center">
          <span className="bg-slate-800 px-3 text-sm text-slate-300">{label}</span>
        </div>
      </div>
    );
  }
  
  return <hr className={classNames('border-t border-slate-600', className)} />;
};

// ===== FILTER DROPDOWNS (checkbox multi-select + single-select) =====
// Shared by Explore > Repositories and Explore > Merge Requests filter bars.
// Same visual/interaction pattern as the local MultiSelectField originally
// built for pages/Reports/TaxonomyReports.tsx.

export type FilterOption = { value: string; label: string; disabled?: boolean };

export const parseMultiFilterValue = (v: string): string[] =>
  (v || '')
    .split(',')
    .map((x) => x.trim())
    .filter(Boolean);

export const removeMultiFilterValue = (current: string, target: string): string =>
  parseMultiFilterValue(current).filter((v) => v !== target).join(',');

interface MultiSelectFieldProps {
  label: string;
  options: FilterOption[];
  value: string;
  onChange: (next: string) => void;
}

interface MultiSelectPanelProps {
  label: string;
  options: FilterOption[];
  value: string;
  onChange: (next: string) => void;
  /** Called after "All X" is picked, so an enclosing popover (if any) can close itself. */
  onRequestClose?: () => void;
}

// The search + "All X" + checkbox-list content of MultiSelectField, without
// the trigger button or open/close state - so callers that need their own
// trigger (e.g. a header filter icon) can reuse the exact same panel instead
// of duplicating this logic. MultiSelectField below is just this panel
// wrapped in its own button + positioning.
export const MultiSelectPanel: React.FC<MultiSelectPanelProps> = ({ label, options, value, onChange, onRequestClose }) => {
  const [query, setQuery] = useState('');
  const selectable = useMemo(() => options.filter((o) => !o.disabled), [options]);
  const selected = parseMultiFilterValue(value);
  const allSelected = selected.length === 0 || selected.length === selectable.length;
  const selectedSet = new Set(allSelected ? selectable.map((o) => o.value) : selected);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return options;
    return options.filter((o) => o.label.toLowerCase().includes(q));
  }, [options, query]);

  const toggle = (target: string) => {
    const next = new Set(allSelected ? selectable.map((o) => o.value) : selected);
    if (next.has(target)) next.delete(target);
    else next.add(target);
    if (next.size === 0 || next.size === selectable.length) {
      onChange('');
      return;
    }
    onChange(Array.from(next).join(','));
  };

  return (
    <div className="space-y-2">
      <input
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder="Search..."
        className="w-full bg-slate-900 border border-slate-700 rounded px-2 py-1.5 text-sm text-slate-100 focus:outline-none focus:ring-2 focus:ring-blue-400"
      />
      <button
        type="button"
        className="w-full text-left text-sm px-2 py-1.5 rounded bg-slate-900 hover:bg-slate-700 text-slate-200"
        onClick={() => {
          onChange('');
          onRequestClose?.();
        }}
      >
        All {label} ({selectable.length})
      </button>
      <div className="max-h-52 overflow-y-auto space-y-1 pr-1">
        {filtered.map((o) => (
          <label
            key={o.value}
            className={`flex items-center gap-2 text-sm px-2 py-1.5 rounded ${o.disabled ? 'text-slate-500 opacity-60 cursor-not-allowed' : 'text-slate-200 hover:bg-slate-700 cursor-pointer'}`}
          >
            <input
              type="checkbox"
              checked={!o.disabled && selectedSet.has(o.value)}
              disabled={o.disabled}
              onChange={() => toggle(o.value)}
              className="accent-blue-500"
            />
            <span className="truncate">{o.label}</span>
          </label>
        ))}
        {filtered.length === 0 && <p className="text-xs text-slate-500 px-2 py-1.5">No matches</p>}
      </div>
    </div>
  );
};

export const MultiSelectField: React.FC<MultiSelectFieldProps> = ({ label, options, value, onChange }) => {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);
  const selected = parseMultiFilterValue(value);
  const allSelected = selected.length === 0 || selected.length === options.length;

  const summary = allSelected
    ? `All ${label}`
    : selected.length === 1
      ? options.find((o) => o.value === selected[0])?.label || selected[0]
      : `${selected.length} selected`;

  useEffect(() => {
    if (!open) return;
    const onClickOutside = (ev: MouseEvent) => {
      if (!rootRef.current) return;
      if (!rootRef.current.contains(ev.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', onClickOutside);
    return () => document.removeEventListener('mousedown', onClickOutside);
  }, [open]);

  return (
    <div ref={rootRef} className="relative w-full">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-label={`Filter by ${label}`}
        className="w-full rounded-lg border border-slate-600 bg-slate-700 px-4 py-2.5 text-left text-sm text-white hover:border-slate-500 focus:outline-none focus:ring-2 focus:ring-blue-400"
      >
        <span className="flex items-center justify-between gap-2">
          <span className="truncate">{summary}</span>
          <span className="text-slate-400">▾</span>
        </span>
      </button>
      {open && (
        <div className="absolute z-30 mt-1 w-full min-w-[14rem] rounded-lg border border-slate-600 bg-slate-800 shadow-xl p-2">
          <MultiSelectPanel label={label} options={options} value={value} onChange={onChange} onRequestClose={() => setOpen(false)} />
        </div>
      )}
    </div>
  );
};

interface SingleSelectFieldProps {
  label: string;
  options: FilterOption[];
  value: string;
  onChange: (next: string) => void;
  /** Set false for exhaustive selectors (e.g. a sort order) where an empty
   * "All X" state doesn't make sense - every option is mutually exclusive
   * and one is always active. Defaults to true (shows "All {label}"). */
  allowClear?: boolean;
}

// Same button/panel chrome as MultiSelectField above, but single-select (no
// checkboxes) - for filters whose options are mutually exclusive rather than
// combinable (e.g. Any/Has open PRs/No open PRs).
export const SingleSelectField: React.FC<SingleSelectFieldProps> = ({ label, options, value, onChange, allowClear = true }) => {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);
  const current = options.find((o) => o.value === value);
  const summary = current ? current.label : `All ${label}`;

  useEffect(() => {
    if (!open) return;
    const onClickOutside = (ev: MouseEvent) => {
      if (!rootRef.current) return;
      if (!rootRef.current.contains(ev.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', onClickOutside);
    return () => document.removeEventListener('mousedown', onClickOutside);
  }, [open]);

  const select = (next: string) => {
    onChange(next);
    setOpen(false);
  };

  return (
    <div ref={rootRef} className="relative w-full">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-label={`Filter by ${label}`}
        className="w-full rounded-lg border border-slate-600 bg-slate-700 px-4 py-2.5 text-left text-sm text-white hover:border-slate-500 focus:outline-none focus:ring-2 focus:ring-blue-400"
      >
        <span className="flex items-center justify-between gap-2">
          <span className="truncate">{summary}</span>
          <span className="text-slate-400">▾</span>
        </span>
      </button>
      {open && (
        <div className="absolute z-30 mt-1 w-full min-w-[14rem] rounded-lg border border-slate-600 bg-slate-800 shadow-xl p-2 space-y-1">
          {allowClear && (
            <button
              type="button"
              className={`w-full text-left text-sm px-2 py-1.5 rounded ${value === '' ? 'bg-blue-600 text-white' : 'bg-slate-900 hover:bg-slate-700 text-slate-200'}`}
              onClick={() => select('')}
            >
              All {label}
            </button>
          )}
          {options.map((o) => (
            <button
              key={o.value}
              type="button"
              className={`w-full text-left text-sm px-2 py-1.5 rounded ${value === o.value ? 'bg-blue-600 text-white' : 'bg-slate-900 hover:bg-slate-700 text-slate-200'}`}
              onClick={() => select(o.value)}
            >
              {o.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
};

// ===== TABS COMPONENT =====
export interface TabItem {
  id: string;
  label: ReactNode;
  badge?: ReactNode;
}

interface TabsProps {
  tabs: TabItem[];
  activeTab: string;
  onChange: (id: string) => void;
  className?: string;
}

export const Tabs: React.FC<TabsProps> = ({ tabs, activeTab, onChange, className }) => {
  return (
    <div className={classNames('flex items-center gap-1 border-b border-slate-700', className)} role="tablist">
      {tabs.map((tab) => {
        const active = tab.id === activeTab;
        return (
          <button
            key={tab.id}
            type="button"
            role="tab"
            aria-selected={active}
            onClick={() => onChange(tab.id)}
            className={classNames(
              'flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors duration-150',
              active
                ? 'border-blue-500 text-white'
                : 'border-transparent text-slate-400 hover:text-slate-200 hover:border-slate-600'
            )}
          >
            {tab.label}
            {tab.badge !== undefined && (
              <span className="rounded-full bg-slate-700 px-2 py-0.5 text-xs text-slate-300">{tab.badge}</span>
            )}
          </button>
        );
      })}
    </div>
  );
};
