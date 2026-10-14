import { useEffect, useState } from 'react';
import { enablePush, pushConfigured, pushState, type PushState } from '../push';

/** Activa los push de este navegador. El permiso solo se pide con un clic del usuario. */
export function BrowserPushCard() {
  const [state, setState] = useState<PushState | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (pushConfigured) void pushState().then(setState);
  }, []);

  if (!pushConfigured || state === null || state === 'unsupported') return null;

  async function enable() {
    setBusy(true);
    setError(null);
    try {
      setState(await enablePush());
    } catch {
      setError('No se pudieron activar las notificaciones. Intenta de nuevo.');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="card" style={{ maxWidth: 640, marginBottom: 14 }} data-testid="browser-push">
      <div className="t">Notificaciones en este navegador</div>
      <div className="s" style={{ marginTop: 4 }}>
        {state === 'granted' && 'Activadas: recibirás avisos aunque la app esté cerrada.'}
        {state === 'default' && 'Recibe avisos aunque la app esté cerrada.'}
        {state === 'denied' && 'Bloqueadas. Actívalas desde la configuración del sitio en tu navegador.'}
      </div>
      {state === 'default' && (
        <button className="btn secondary" style={{ marginTop: 10 }} onClick={() => void enable()} disabled={busy}>
          {busy ? 'Activando…' : 'Activar'}
        </button>
      )}
      {error && <div className="s" style={{ color: 'var(--danger, #b42318)', marginTop: 8 }}>{error}</div>}
    </div>
  );
}
