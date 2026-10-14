import { fmtDay, fmtTime } from '../../../shared/format';
import { GROUP_LABEL, type AppNotification } from '../api';

/** Figma: componente "Notification item" (variantes Unread / Read). */
export function NotificationItem({ n, onOpen }: { n: AppNotification; onOpen: (n: AppNotification) => void }) {
  return (
    <button
      type="button"
      className={`notif-item${n.is_read ? '' : ' unread'}`}
      onClick={() => onOpen(n)}
      data-testid="notification-item"
    >
      <span className="notif-dot" aria-hidden />
      <span className="notif-body">
        <span className="notif-title">
          {n.title}
          {!n.is_read && <span className="sr-only"> (no leída)</span>}
        </span>
        <span className="notif-text">{n.body}</span>
        <span className="notif-meta">
          {fmtDay(n.created_at)} · {fmtTime(n.created_at)} · {GROUP_LABEL[n.group] ?? n.group}
        </span>
      </span>
    </button>
  );
}
