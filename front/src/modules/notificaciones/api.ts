import { api } from '../../shared/api/client';

/** Contrato: back/notificaciones/API.md */

export type NotificationGroup =
  | 'ACCESS_CONTROL'
  | 'INTERCOM'
  | 'COMMUNITY'
  | 'RESERVATIONS'
  | 'ASSEMBLIES'
  | 'VOTES'
  | 'SYSTEM';

export type DeliveryStatus = 'INBOX_ONLY' | 'PUSH_SENT' | 'PUSH_PARTIAL' | 'PUSH_FAILED';

export interface AppNotification {
  id: string;
  complex_id?: string;
  user_id: string;
  title: string;
  body: string;
  data: Record<string, unknown>;
  type: string;
  group: NotificationGroup;
  is_read: boolean;
  read_at?: string;
  delivery_status: DeliveryStatus;
  delivered_at?: string;
  created_at: string;
}

export interface PreferenceItem {
  group: NotificationGroup;
  enabled: boolean;
  locked: boolean;
}

export const GROUP_LABEL: Record<NotificationGroup, string> = {
  ACCESS_CONTROL: 'Control de acceso',
  INTERCOM: 'Citofonía',
  COMMUNITY: 'Comunidad',
  RESERVATIONS: 'Reservas',
  ASSEMBLIES: 'Asambleas',
  VOTES: 'Votaciones',
  SYSTEM: 'Sistema',
};

export interface DeviceRegistration {
  platform: 'IOS' | 'ANDROID' | 'WEB';
  push_token: string;
  device_name?: string;
  app_version?: string;
}

export interface InboxQuery {
  unread?: boolean;
  groups?: NotificationGroup[];
  limit?: number;
}

export const notificationsApi = {
  list: ({ unread, groups, limit }: InboxQuery = {}) => {
    const q = new URLSearchParams();
    if (unread) q.set('unread', 'true');
    if (groups?.length) q.set('group', groups.join(','));
    if (limit) q.set('limit', String(limit));
    const qs = q.toString();
    return api<AppNotification[]>(`/users/me/notifications${qs ? `?${qs}` : ''}`);
  },
  unreadCount: () => api<{ count: number }>('/users/me/notifications/unread-count').then((r) => r.count),
  markRead: (id: string) => api<void>(`/notifications/${id}/read`, { method: 'PATCH' }),
  markAllRead: () => api<{ marked_count: number }>('/notifications/mark-all-read', { method: 'POST' }),
  preferences: () => api<{ preferences: PreferenceItem[] }>('/me/notification-preferences').then((r) => r.preferences),
  setPreference: (group: NotificationGroup, enabled: boolean) =>
    api<{ preferences: PreferenceItem[] }>('/me/notification-preferences', {
      method: 'PUT',
      body: { preferences: [{ group, enabled }] },
    }).then((r) => r.preferences),
  registerDevice: (device: DeviceRegistration) => api<unknown>('/me/devices', { method: 'POST', body: device }),
  unregisterDevice: (push_token: string) =>
    api<void>('/me/devices/unregister', { method: 'POST', body: { push_token } }),
};
