import { forwardRef, type ButtonHTMLAttributes } from 'react';
import { LoaderCircle } from 'lucide-react';
export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> { variant?: 'primary' | 'secondary' | 'text' | 'danger'; loading?: boolean; }
export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button({ variant = 'primary', loading = false, disabled, className = '', children, ...props }, ref) {
  return <button ref={ref} className={`button button--${variant} ${className}`} disabled={disabled || loading} aria-busy={loading || undefined} {...props}>
    {loading && <LoaderCircle className="loading-spinner" size={18} aria-hidden="true" />}{children}
  </button>;
});
