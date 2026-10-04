import type { ReactNode } from 'react';

type Tone = 'green' | 'amber' | 'red' | 'gray';

export function Badge({ tone, children }: { tone: Tone; children: ReactNode }) {
  return <span className={`badge ${tone}`}>{children}</span>;
}

export function PageHead({ title, subtitle, action }: { title: string; subtitle?: string; action?: ReactNode }) {
  return (
    <div className={`page-head${action ? ' row' : ''}`}>
      <div>
        <h1>{title}</h1>
        {subtitle && <p>{subtitle}</p>}
      </div>
      {action}
    </div>
  );
}

export function Loading() {
  return <div className="spinner">Cargando…</div>;
}

export function ErrorBox({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="alert red">
      <b>No se pudo completar la operación</b>
      {message}
      {onRetry && (
        <div className="btn-row">
          <button className="btn danger-soft" onClick={onRetry}>
            Reintentar
          </button>
        </div>
      )}
    </div>
  );
}

export function Field({
  label,
  full,
  error,
  children,
}: {
  label: string;
  full?: boolean;
  error?: string;
  children: ReactNode;
}) {
  return (
    <div className={`field${full ? ' full' : ''}`}>
      <label>{label}</label>
      {children}
      {error && <span className="err">{error}</span>}
    </div>
  );
}
