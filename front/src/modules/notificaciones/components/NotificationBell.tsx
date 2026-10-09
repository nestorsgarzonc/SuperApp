import { Link } from 'react-router-dom';
import { useUnreadCount } from '../unread';

/** Campana del encabezado con el contador de no leídas. */
export function NotificationBell() {
  const count = useUnreadCount();
  const label = count ? `Notificaciones, ${count} sin leer` : 'Notificaciones';
  return (
    <Link to="/notificaciones" className="bell" aria-label={label} title={label} data-testid="notification-bell">
      <span aria-hidden>🔔</span>
      {!!count && <span className="bell-count">{count > 99 ? '99+' : count}</span>}
    </Link>
  );
}
