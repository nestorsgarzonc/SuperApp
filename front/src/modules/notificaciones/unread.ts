import { useEffect, useSyncExternalStore } from 'react';
import { getSession } from '../../shared/api/client';
import { notificationsApi } from './api';

// Contador de no leídas compartido por la campana del header, el centro de
// notificaciones y el dashboard: una sola consulta periódica para toda la app.

const POLL_MS = 30_000;

let count: number | null = null;
let sessionKey = '';
const listeners = new Set<() => void>();
let timer: ReturnType<typeof setInterval> | undefined;

const emit = () => listeners.forEach((l) => l());

const keyOf = () => {
  const s = getSession();
  return s ? `${s.userId}:${s.complexId}` : '';
};

export async function refreshUnread() {
  const key = keyOf();
  if (!key) return;
  try {
    const next = await notificationsApi.unreadCount();
    if (keyOf() !== key) return; // cambió la sesión mientras tanto
    sessionKey = key;
    count = next;
  } catch {
    // Sin conexión o servicio caído: se conserva el último valor conocido.
  }
  emit();
}

/** Ajuste optimista tras marcar como leídas, antes de que responda el servidor. */
export function adjustUnread(delta: number) {
  if (count === null) return;
  count = Math.max(0, count + delta);
  emit();
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  if (listeners.size === 1) {
    void refreshUnread();
    timer = setInterval(() => void refreshUnread(), POLL_MS);
  }
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0) clearInterval(timer);
  };
}

const snapshot = () => (sessionKey === keyOf() ? count : null);

export function useUnreadCount() {
  const value = useSyncExternalStore(subscribe, snapshot);
  const key = keyOf();
  // Al cambiar de conjunto o de usuario el contador anterior deja de valer.
  useEffect(() => {
    void refreshUnread();
  }, [key]);
  return value;
}
