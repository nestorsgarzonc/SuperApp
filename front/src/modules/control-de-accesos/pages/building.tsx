import { useRef, useState } from 'react';
import { useAsync } from '../../../shared/useAsync';
import { ApiError } from '../../../shared/api/client';
import { fmtDay, fmtTime } from '../../../shared/format';
import { uuid } from '../../../shared/uuid';
import { Badge, ErrorBox, Loading, PageHead } from '../../../shared/ui';
import { residentApi, type BuildingStatus } from '../api';
import { MOVEMENT_LABEL } from '../accessLog';

const STATUS_COPY: Record<BuildingStatus, { title: string; tone: 'green' | 'gray' | 'amber' }> = {
  INSIDE: { title: 'Estás dentro del edificio', tone: 'green' },
  OUTSIDE: { title: 'Estás fuera del edificio', tone: 'gray' },
  UNKNOWN: { title: 'Aún no has registrado movimientos', tone: 'amber' },
};

/** Residente: marca que entra o sale del edificio y ve su propio historial. */
export function BuildingPage() {
  const { data, error, loading, reload } = useAsync(() => residentApi.me(), []);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  // Un mismo client_id por intento: si falla la red y reintentas, el servidor no duplica.
  const pendingId = useRef<{ movement: 'ENTRY' | 'EXIT'; id: string } | null>(null);

  async function move(movement: 'ENTRY' | 'EXIT') {
    setBusy(true);
    setNotice(null);
    setActionError(null);
    if (pendingId.current?.movement !== movement) pendingId.current = { movement, id: uuid() };
    try {
      const entry = await residentApi.move(movement, pendingId.current.id);
      pendingId.current = null;
      setNotice(`${movement === 'ENTRY' ? 'Ingreso' : 'Salida'} registrado · ${fmtTime(entry.occurred_at)}`);
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) pendingId.current = null; // el estado cambió: otro intento nuevo
      setActionError((e as Error).message);
    } finally {
      setBusy(false);
      reload();
    }
  }

  if (loading && !data) return <Loading />;
  if (error && !data) return <ErrorBox message={error} onRetry={reload} />;
  if (!data) return null;

  const copy = STATUS_COPY[data.status];
  const inside = data.status === 'INSIDE';
  const outside = data.status === 'OUTSIDE';

  return (
    <>
      <PageHead title="Mi acceso al edificio" subtitle="Registra cuándo entras y cuándo sales" />

      <div className="card" data-testid="building-status">
        <Badge tone={copy.tone}>{inside ? '● Dentro' : outside ? '● Fuera' : '● Sin registros'}</Badge>
        <h2 style={{ fontSize: 22, margin: '10px 0 2px' }}>{copy.title}</h2>
        {data.since && (
          <div className="s" style={{ color: 'var(--muted)' }}>
            Desde {fmtDay(data.since)}, {fmtTime(data.since)}
          </div>
        )}
        <div className="btn-row">
          <button className="btn" disabled={busy || inside} onClick={() => void move('ENTRY')}>
            Ingresar al edificio
          </button>
          <button className="btn secondary" disabled={busy || outside} onClick={() => void move('EXIT')}>
            Salir del edificio
          </button>
        </div>
      </div>

      {notice && <div className="alert green" style={{ marginTop: 14 }}>✓ {notice}</div>}
      {actionError && (
        <div style={{ marginTop: 14 }}>
          <ErrorBox message={actionError} />
        </div>
      )}

      <div className="card">
        <h2>Mis movimientos</h2>
        {data.movements.length === 0 ? (
          <div className="empty">Todavía no has registrado entradas ni salidas.</div>
        ) : (
          <ul className="list">
            {data.movements.map((m) => (
              <li key={m.client_id}>
                <div>
                  <div className="t">{m.movement === 'ENTRY' ? 'Ingreso al edificio' : 'Salida del edificio'}</div>
                  <div className="s">
                    {fmtDay(m.occurred_at)} · {fmtTime(m.occurred_at)} · {m.unit_label}
                  </div>
                </div>
                <Badge tone={m.movement === 'ENTRY' ? 'green' : 'gray'}>{MOVEMENT_LABEL[m.movement]}</Badge>
              </li>
            ))}
          </ul>
        )}
      </div>
    </>
  );
}
