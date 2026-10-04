import { Link } from 'react-router-dom';
import { useAuth } from '../../../shared/auth/AuthContext';
import { useAsync } from '../../../shared/useAsync';
import { useOnline } from '../../../shared/useOnline';
import { fmtRange, fmtTime } from '../../../shared/format';
import { Badge, ErrorBox, Loading, PageHead } from '../../../shared/ui';
import { accessApi } from '../api';
import { movementLabel, MOVEMENT_LABEL, syncPending, useAccessLog, type AccessLogEntry } from '../accessLog';
import { PresenceCard } from '../components/PresenceCard';
import { QrSystemNotice } from '../components/QrSystemNotice';
import { useQrSystem } from '../useQrSystem';
import { GrantBadge, ResultBadge } from '../components/badges';
import { VISITOR_TYPE_LABEL, grantView } from '../types';

export function HomePage() {
  const { profile } = useAuth();
  if (profile?.role === 'GUARD') return <GuardHome />;
  if (profile?.role === 'ADMIN') return <AdminHome />;
  return <ResidentHome />;
}

function Tile({ to, icon, label, primary }: { to: string; icon: string; label: string; primary?: boolean }) {
  return (
    <Link to={to} className={`action-tile${primary ? ' primary' : ''}`}>
      <span className="ico">{icon}</span>
      {label}
    </Link>
  );
}

function ResidentHome() {
  const { session, profile } = useAuth();
  const qr = useQrSystem();
  const { data, loading, error, reload } = useAsync(
    () => (session!.unitId ? accessApi.unitGrants(session!.unitId) : Promise.resolve([])),
    [session?.unitId],
  );
  const active = (data ?? []).filter((g) => ['active', 'upcoming'].includes(grantView(g))).slice(0, 5);

  return (
    <>
      <PageHead title={`Hola, ${profile!.name.split(' ')[0]}`} subtitle={`${session!.complexName} · ${profile!.subtitle}`} />
      <div style={{ marginBottom: 14 }}>
        <QrSystemNotice qr={qr} audience="resident" />
      </div>
      <div className="actions-grid" style={{ marginBottom: 14 }}>
        <Tile primary to="/autorizaciones/nueva" icon="+" label="Autorizar visitante" />
        <Tile to="/autorizaciones" icon="◇" label="Mis autorizaciones" />
        <Tile to="/edificio" icon="⇄" label="Entrar / salir del edificio" />
      </div>
      <div className="card">
        <h2>Autorizaciones activas</h2>
        {loading && <Loading />}
        {error && <ErrorBox message={error} onRetry={reload} />}
        {!loading && !error && active.length === 0 && <div className="empty">No tienes autorizaciones activas.</div>}
        <ul className="list">
          {active.map((g) => (
            <li key={g.id}>
              <Link to={`/autorizaciones/${g.id}`} style={{ flex: 1 }}>
                <div className="t">
                  {g.visitor_name} · {VISITOR_TYPE_LABEL[g.visitor_type]}
                </div>
                <div className="s">{fmtRange(g.valid_from ?? g.created_at, g.valid_until)}</div>
              </Link>
              <GrantBadge grant={g} />
            </li>
          ))}
        </ul>
      </div>
    </>
  );
}

function RecentList({ entries }: { entries: AccessLogEntry[] }) {
  if (!entries.length) return <div className="empty">Aún no hay movimientos registrados.</div>;
  return (
    <ul className="list">
      {entries.map((e) => (
        <li key={e.id}>
          <div>
            <div className="t">
              {e.visitor} · {e.unit}
            </div>
            <div className="s">
              {fmtTime(e.at)} · {movementLabel(e)}
            </div>
          </div>
          <ResultBadge result={e.result} />
        </li>
      ))}
    </ul>
  );
}

function GuardHome() {
  const { profile, session } = useAuth();
  const qr = useQrSystem();
  const online = useOnline();
  const { entries, pending, syncing, error } = useAccessLog();

  return (
    <>
      <PageHead title="Portería" subtitle={`${profile!.subtitle} · ${session!.complexName}`} />
      <div className="status-bar">
        <span>Operación en tiempo real</span>
        {online ? (
          <Badge tone="green">● Conectado</Badge>
        ) : (
          <Badge tone="red">● Sin conexión</Badge>
        )}
      </div>
      <div style={{ marginBottom: 14 }}>
        <QrSystemNotice qr={qr} audience="guard" />
      </div>
      <div className="actions-grid" style={{ marginBottom: 14 }}>
        <Tile primary to="/porteria/visitante" icon="+" label="REGISTRAR VISITANTE" />
        <Tile primary to="/porteria/validar" icon="◆" label="VALIDAR PASE" />
        <Tile to="/porteria/movimiento" icon="⇥" label="REGISTRAR INGRESO" />
        <Tile to="/porteria/movimiento" icon="⇤" label="REGISTRAR SALIDA" />
      </div>

      {!online && (
        <div className="alert red" style={{ marginBottom: 14 }}>
          <b>⊘ Sin conexión · Modo offline</b>
          Los registros se guardan en este equipo y se sincronizarán al recuperar la conexión.
        </div>
      )}
      {online && pending.length > 0 && (
        <div className="alert blue" style={{ marginBottom: 14 }}>
          <b>⟳ {syncing ? 'Sincronizando…' : 'Registros por sincronizar'}</b>
          {pending.length} registro(s) pendiente(s) de sincronizar.
          <div className="btn-row">
            <button className="btn secondary" onClick={() => void syncPending()}>
              Sincronizar ahora
            </button>
          </div>
        </div>
      )}

      {error && entries.length === 0 && <ErrorBox message={error} />}
      <PresenceCard />
      <div className="card">
        <h2>Accesos recientes</h2>
        <RecentList entries={entries.slice(0, 5)} />
      </div>

      {pending.length > 0 && (
        <div className="card">
          <h2>Operaciones guardadas localmente</h2>
          <ul className="list">
            {pending.slice(0, 8).map((e) => (
              <li key={e.id}>
                <div>
                  <div className="t">
                    {MOVEMENT_LABEL[e.movement]} · {e.visitor} · {e.unit}
                  </div>
                  <div className="s">{fmtTime(e.at)} · Pendiente de sincronizar</div>
                </div>
                <Badge tone="amber">▲ Pendiente</Badge>
              </li>
            ))}
          </ul>
        </div>
      )}
    </>
  );
}

function Stat({ label, value, note, danger }: { label: string; value: number | string; note?: string; danger?: boolean }) {
  return (
    <div className="card" style={{ margin: 0 }}>
      <div style={{ fontWeight: 600, fontSize: 14 }}>{label}</div>
      <div style={{ fontSize: 34, fontWeight: 700 }}>{value}</div>
      {note && <div style={{ fontSize: 13, color: danger ? 'var(--red)' : 'var(--muted)' }}>{note}</div>}
    </div>
  );
}

function AdminHome() {
  const { session } = useAuth();
  const { entries: log, error: logError } = useAccessLog();
  const grants = useAsync(() => accessApi.summary(), []);

  const today = new Date().toDateString();
  const todays = log.filter((e) => new Date(e.at).toDateString() === today);
  const denied = todays.filter((e) => e.result === 'DENIED').length;

  return (
    <>
      <PageHead title="Panel de administración" subtitle={`Resumen de ${session!.complexName}`} />
      {grants.error && <ErrorBox message={grants.error} onRetry={grants.reload} />}
      {logError && <ErrorBox message={logError} />}
      <div className="actions-grid" style={{ marginBottom: 14 }}>
        <Stat label="Autorizaciones" value={grants.loading ? '…' : (grants.data?.total ?? 0)} note={`${grants.data?.active ?? 0} activas ahora`} />
        <Stat label="Accesos hoy" value={todays.length} note={`${denied} denegados`} danger={denied > 0} />
      </div>
      <PresenceCard />
      <div className="card">
        <h2>Actividad reciente</h2>
        <RecentList entries={log.slice(0, 6)} />
        <div className="btn-row">
          <Link className="btn secondary" to="/historial">
            Ver historial completo
          </Link>
        </div>
      </div>
    </>
  );
}
