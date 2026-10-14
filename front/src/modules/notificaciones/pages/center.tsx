import { useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { useAsync } from '../../../shared/useAsync';
import { ErrorBox, Loading, PageHead } from '../../../shared/ui';
import { notificationsApi, type AppNotification, type NotificationGroup } from '../api';
import { NotificationItem } from '../components/NotificationItem';
import { adjustUnread, refreshUnread, useUnreadCount } from '../unread';

type FilterId = 'all' | 'unread' | 'access' | 'community' | 'reservations' | 'assemblies' | 'system';

/** Chips del diseño → query del API (ver back/notificaciones/API.md). */
const FILTERS: { id: FilterId; label: string; groups?: NotificationGroup[] }[] = [
  { id: 'all', label: 'Todas' },
  { id: 'unread', label: 'No leídas' },
  { id: 'access', label: 'Control de acceso', groups: ['ACCESS_CONTROL', 'INTERCOM'] },
  { id: 'community', label: 'Comunidad', groups: ['COMMUNITY'] },
  { id: 'reservations', label: 'Reservas', groups: ['RESERVATIONS'] },
  { id: 'assemblies', label: 'Asambleas', groups: ['ASSEMBLIES', 'VOTES'] },
  { id: 'system', label: 'Sistema', groups: ['SYSTEM'] },
];

const isFilter = (v: string | null): v is FilterId => FILTERS.some((f) => f.id === v);

export function NotificationCenterPage() {
  const nav = useNavigate();
  const [params, setParams] = useSearchParams();
  const raw = params.get('filtro');
  const filter: FilterId = isFilter(raw) ? raw : 'all';
  const current = FILTERS.find((f) => f.id === filter)!;
  const unread = useUnreadCount();
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  const { data, error, loading, reload } = useAsync(
    () => notificationsApi.list({ unread: filter === 'unread', groups: current.groups, limit: 100 }),
    [filter],
  );
  // Copia local para pintar "leída" al instante, sin recargar la lista.
  const [readIds, setReadIds] = useState<Set<string>>(new Set());
  const items = (data ?? []).map((n) => (readIds.has(n.id) ? { ...n, is_read: true } : n));

  function select(id: FilterId) {
    setParams(id === 'all' ? {} : { filtro: id }, { replace: true });
  }

  function open(n: AppNotification) {
    if (!n.is_read) {
      setReadIds((s) => new Set(s).add(n.id));
      adjustUnread(-1);
      notificationsApi.markRead(n.id).catch(() => void refreshUnread());
    }
    const link = typeof n.data?.deep_link === 'string' ? n.data.deep_link : null;
    if (link?.startsWith('/')) nav(link);
  }

  async function markAll() {
    setBusy(true);
    setActionError(null);
    try {
      await notificationsApi.markAllRead();
      setReadIds(new Set((data ?? []).map((n) => n.id)));
      await refreshUnread();
      if (filter === 'unread') reload();
    } catch (e) {
      setActionError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <PageHead
        title="Notificaciones"
        subtitle="Reservas, novedades, anuncios y control de acceso"
        action={
          <div className="btn-row" style={{ marginTop: 0 }}>
            <Link className="btn ghost" to="/notificaciones/preferencias">
              Preferencias
            </Link>
            <button className="btn secondary" onClick={() => void markAll()} disabled={busy || unread === 0}>
              {busy ? 'Marcando…' : 'Marcar todas como leídas'}
            </button>
          </div>
        }
      />

      <div className="pills" role="group" aria-label="Filtrar notificaciones">
        {FILTERS.map((f) => (
          <button
            key={f.id}
            className={`pill${filter === f.id ? ' active' : ''}`}
            aria-pressed={filter === f.id}
            onClick={() => select(f.id)}
          >
            {f.label}
            {f.id === 'unread' && unread !== null && ` (${unread})`}
          </button>
        ))}
      </div>

      {actionError && (
        <div style={{ marginBottom: 14 }}>
          <ErrorBox message={actionError} />
        </div>
      )}
      {loading && !data && <Loading />}
      {error && <ErrorBox message={error} onRetry={reload} />}
      {data && items.length === 0 && (
        <div className="empty">
          {filter === 'unread' ? 'No tienes notificaciones sin leer.' : 'No hay notificaciones en esta categoría.'}
        </div>
      )}
      <div className="notif-list">
        {items.map((n) => (
          <NotificationItem key={n.id} n={n} onOpen={open} />
        ))}
      </div>
    </>
  );
}
