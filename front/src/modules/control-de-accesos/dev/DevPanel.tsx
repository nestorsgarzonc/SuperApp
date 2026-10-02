import { useCallback, useEffect, useState } from 'react';
import { api } from '../../../shared/api/client';

// SOLO PRUEBAS (ver devApi.ts). Simula la caída del sistema QR (lector/servicio
// externo) para ver el circuit breaker en acción: caído = deja de responder.

interface QrDetail {
  mode?: 'UP' | 'DOWN';
  state: 'CLOSED' | 'OPEN' | 'HALF_OPEN';
  failures: number;
  retry_after_seconds: number;
  simulated: boolean;
}

const STATE_COPY = { CLOSED: 'cerrado', OPEN: 'ABIERTO', HALF_OPEN: 'semiabierto' } as const;

export function DevPanel() {
  const [d, setD] = useState<QrDetail | null>(null);
  // Plegado por defecto: desplegado tapa botones de la app en pantallas chicas.
  const [open, setOpen] = useState(false);
  const [error, setError] = useState(false);

  const load = useCallback(async () => {
    try {
      setD(await api<QrDetail>('/dev/qr-system', { auth: false }));
      setError(false);
    } catch {
      setError(true);
    }
  }, []);

  useEffect(() => {
    void load();
    const t = setInterval(() => void load(), 2000);
    return () => clearInterval(t);
  }, [load]);

  async function setMode(mode: 'UP' | 'DOWN') {
    setD(await api<QrDetail>('/dev/qr-system', { method: 'PUT', body: { mode }, auth: false }));
  }
  async function resetBreaker() {
    setD(await api<QrDetail>('/dev/qr-system/reset', { method: 'POST', auth: false }));
  }

  if (error || !d?.simulated) return null;
  const down = d.mode === 'DOWN';

  return (
    <aside className="devpanel" data-testid="dev-panel" aria-label="Herramientas de desarrollo">
      <button className="devpanel-head" onClick={() => setOpen((o) => !o)} aria-expanded={open}>
        🛠 QR: <b data-testid="dev-qr-mode">{down ? 'CAÍDO' : 'operativo'}</b> · circuito{' '}
        <b data-testid="dev-qr-state">{STATE_COPY[d.state]}</b> {open ? '▾' : '▸'}
      </button>
      {open && (
        <div className="devpanel-body">
          <div>
            {d.state === 'OPEN' && d.retry_after_seconds > 0 && <>Se puede reintentar en {d.retry_after_seconds}s. </>}
            {d.failures > 0 && d.state === 'CLOSED' && <>{d.failures} fallo(s) seguido(s). </>}
            {d.state === 'CLOSED' && d.failures === 0 && <>Sin fallos.</>}
          </div>
          <div className="devpanel-row">
            {down ? (
              <button className="btn secondary" onClick={() => void setMode('UP')}>
                Restablecer sistema QR
              </button>
            ) : (
              <button className="btn danger" onClick={() => void setMode('DOWN')}>
                Simular caída del sistema QR
              </button>
            )}
            {d.state !== 'CLOSED' && (
              <button className="btn outline" onClick={() => void resetBreaker()}>
                Cerrar circuito ya
              </button>
            )}
          </div>
          <small>Caído = no responde. El breaker lo corta por timeout y abre el circuito.</small>
        </div>
      )}
    </aside>
  );
}
