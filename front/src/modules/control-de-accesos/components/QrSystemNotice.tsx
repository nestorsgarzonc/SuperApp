import type { ReactNode } from 'react';
import type { QrSystem } from '../useQrSystem';

const COPY = {
  resident: {
    title: 'El sistema de QR no está funcionando',
    body: 'Por ahora autoriza a tus visitantes por documento (pre-autorización): portería los validará con su número de documento.',
  },
  guard: {
    title: 'El sistema de QR no responde',
    body: 'No se pueden emitir ni leer códigos QR. Valida los ingresos por documento (pre-autorización).',
  },
};

/** Aviso cuando el circuit breaker del sistema QR está abierto; no pinta nada si funciona. */
export function QrSystemNotice({ qr, audience, children }: { qr: QrSystem; audience: 'resident' | 'guard'; children?: ReactNode }) {
  if (qr.available) return null;
  const copy = COPY[audience];
  return (
    <div className="alert amber" role="status" data-testid="qr-down-notice">
      <b>⚠ {copy.title}</b>
      {copy.body}
      {qr.retry_after_seconds ? (
        <div style={{ marginTop: 4, fontSize: 13 }}>Se reintentará automáticamente; vuelve a probar en unos {qr.retry_after_seconds} s.</div>
      ) : null}
      {children}
    </div>
  );
}
