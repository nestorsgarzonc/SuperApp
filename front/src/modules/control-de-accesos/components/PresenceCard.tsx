import { useAsync } from '../../../shared/useAsync';
import { fmtDay, fmtTime } from '../../../shared/format';
import { ErrorBox, Loading } from '../../../shared/ui';
import { residentApi } from '../api';

/** Portería/administración: residentes que están dentro del edificio ahora. */
export function PresenceCard() {
  const { data, error, loading, reload } = useAsync(() => residentApi.presence(), []);

  return (
    <div className="card" data-testid="presence-card">
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 12 }}>
        <h2 style={{ margin: 0 }}>Residentes dentro del edificio{data ? ` (${data.length})` : ''}</h2>
        <button className="btn ghost" onClick={reload} aria-label="Actualizar">
          ⟳
        </button>
      </div>
      {loading && !data && <Loading />}
      {error && <ErrorBox message={error} onRetry={reload} />}
      {data && data.length === 0 && <div className="empty">No hay residentes dentro en este momento.</div>}
      {data && data.length > 0 && (
        <ul className="list">
          {data.map((p) => (
            <li key={p.user_id}>
              <div>
                <div className="t">{p.name}</div>
                <div className="s">
                  {p.unit_label} · desde {fmtDay(p.since)}, {fmtTime(p.since)}
                </div>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
