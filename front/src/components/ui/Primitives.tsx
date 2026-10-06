import { forwardRef, type HTMLAttributes, type InputHTMLAttributes, type ReactNode } from 'react';
import { LoaderCircle, X } from 'lucide-react';

type BadgeTone = 'primary' | 'success' | 'warning' | 'error' | 'info' | 'neutral';
export const Badge = forwardRef<HTMLSpanElement, HTMLAttributes<HTMLSpanElement> & { tone?: BadgeTone }>(function Badge({ tone = 'neutral', className = '', children, ...props }, ref) {
  return <span ref={ref} className={`ui-badge ui-badge--${tone} ${className}`} {...props}>{children}</span>;
});

export const Card = forwardRef<HTMLElement, HTMLAttributes<HTMLElement>>(function Card({ className = '', ...props }, ref) {
  return <article ref={ref} className={`ui-card ${className}`} {...props} />;
});

export const Alert = forwardRef<HTMLDivElement, HTMLAttributes<HTMLDivElement> & { tone?: Exclude<BadgeTone, 'neutral' | 'primary'>; icon?: ReactNode }>(function Alert({ tone = 'info', icon, className = '', children, role, ...props }, ref) {
  return <div ref={ref} className={`alert-box alert-box--${tone} ${className}`} role={role ?? (tone === 'error' ? 'alert' : 'status')} {...props}>{icon && <span className="alert-box__icon" aria-hidden="true">{icon}</span>}<div>{children}</div></div>;
});

export const Spinner = forwardRef<HTMLSpanElement, HTMLAttributes<HTMLSpanElement> & { label?: string }>(function Spinner({ label = 'Cargando', className = '', ...props }, ref) {
  return <span ref={ref} className={`spinner-wrap ${className}`} role="status" aria-label={label} {...props}><LoaderCircle className="loading-spinner" aria-hidden="true" /></span>;
});

export const Toast = forwardRef<HTMLDivElement, HTMLAttributes<HTMLDivElement> & { onDismiss?: () => void }>(function Toast({ onDismiss, className = '', children, ...props }, ref) {
  return <div ref={ref} className={`toast ${className}`} role="status" {...props}><span>{children}</span>{onDismiss && <button className="toast__close" type="button" aria-label="Cerrar notificación" onClick={onDismiss}><X size={17}/></button>}</div>;
});

export const Checkbox = forwardRef<HTMLInputElement, Omit<InputHTMLAttributes<HTMLInputElement>, 'type'> & { label: string }>(function Checkbox({ label, className = '', ...props }, ref) {
  return <label className={`check-row ${className}`}><input ref={ref} type="checkbox" {...props}/><span className="check-box" aria-hidden="true"/><span>{label}</span></label>;
});
