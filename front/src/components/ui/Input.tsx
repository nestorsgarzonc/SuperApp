import { forwardRef, type InputHTMLAttributes, type ReactNode } from 'react';
export interface InputProps extends InputHTMLAttributes<HTMLInputElement> { label: string; hint?: string; error?: string; trailing?: ReactNode; }
export const Input = forwardRef<HTMLInputElement, InputProps>(function Input({ label, hint, error, id, className = '', trailing, ...props }, ref) {
  const inputId = id ?? props.name;
  const hintId = `${inputId}-hint`;
  const errorId = `${inputId}-error`;
  return <div className="field">
    <label className="field__label" htmlFor={inputId}>{label}</label>
    <div className={`field__control ${error ? 'field__control--error' : ''}`}>
      <input ref={ref} id={inputId} className={`field__input ${className}`} aria-invalid={Boolean(error)} aria-describedby={[hint ? hintId : '', error ? errorId : ''].filter(Boolean).join(' ') || undefined} {...props} />
      {trailing}
    </div>
    {hint && !error && <p className="field__hint" id={hintId}>{hint}</p>}
    {error && <p className="field__error" id={errorId} role="alert"><span aria-hidden="true">✕</span> {error}</p>}
  </div>;
});
