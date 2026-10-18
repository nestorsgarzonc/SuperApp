import { useEffect, useSyncExternalStore } from 'react';
import { api, ApiError, getSession } from '../../shared/api/client';

// ───────────────────────────────────────────────────────────────────────────
// Bitácora de ingresos/salidas/intentos (POST/GET /access-logs).
//
// Offline-first: cada registro nace en una cola local (`pending`) con un
// client_id propio y se envía al ms apenas hay red. El endpoint es idempotente
// por client_id, así que reintentar nunca duplica. Lo ya sincronizado se lee
// del servidor (`remote`).
// ───────────────────────────────────────────────────────────────────────────

export type Movement = 'ENTRY' | 'EXIT' | 'ATTEMPT';
export type LogResult = 'AUTHORIZED' | 'DENIED';

export const MOVEMENT_LABEL: Record<Movement, string> = { ENTRY: 'Ingreso', EXIT: 'Salida', ATTEMPT: 'Intento' };

export type Subject = 'VISITOR' | 'RESIDENT';

/** "Ingreso al edificio" para residentes; "Ingreso" para visitantes. */
export const movementLabel = (e: { movement: Movement; subject?: Subject }) =>
  e.subject === 'RESIDENT' ? (e.movement === 'ENTRY' ? 'Ingreso al edificio' : 'Salida del edificio') : MOVEMENT_LABEL[e.movement];

export interface AccessLogEntry {
  /** = client_id */
  id: string;
  at: string;
  movement: Movement;
  result: LogResult;
  visitor: string;
  unit: string;
  visitorType: string;
  vehicle?: string;
  registeredBy: string;
  grantId?: string;
  synced: boolean;
  subject?: Subject;
  /** Dueño de un registro en cola: solo se envía con la sesión (conjunto + persona) que lo creó. */
  complexId?: string;
  userId?: string;
}

interface ApiLog {
  client_id: string;
  subject_type?: Subject;
  movement: Movement;
  result: LogResult;
  visitor_name: string;
  unit_label: string;
  visitor_type?: string;
  vehicle?: string;
  grant_id?: string;
  registered_by_name: string;
  occurred_at: string;
}

const fromApi = (l: ApiLog): AccessLogEntry => ({
  id: l.client_id,
  at: l.occurred_at,
  movement: l.movement,
  result: l.result,
  visitor: l.visitor_name,
  unit: l.unit_label,
  visitorType: l.visitor_type ?? '—',
  vehicle: l.vehicle,
  registeredBy: l.registered_by_name || '—',
  grantId: l.grant_id,
  synced: true,
  subject: l.subject_type,
});

const toApi = (e: AccessLogEntry) => ({
  client_id: e.id,
  movement: e.movement,
  result: e.result,
  visitor_name: e.visitor,
  unit_label: e.unit,
  visitor_type: e.visitorType === '—' ? undefined : e.visitorType,
  vehicle: e.vehicle,
  grant_id: e.grantId,
  occurred_at: e.at,
});

const KEY = 'superapp.accessLog.pending';

interface State {
  /** Cola completa (de todos los conjuntos/personas de este equipo). */
  queue: AccessLogEntry[];
  /** Solo la cola de la sesión actual. */
  pending: AccessLogEntry[];
  remote: AccessLogEntry[];
  /** Conjunto del que se descargó `remote`. */
  remoteComplexId: string | null;
  loading: boolean;
  error: string | null;
  syncing: boolean;
  merged: AccessLogEntry[];
}

const loadPending = (): AccessLogEntry[] => {
  try {
    return JSON.parse(localStorage.getItem(KEY) ?? '[]') as AccessLogEntry[];
  } catch {
    return [];
  }
};

const mine = (e: AccessLogEntry) => {
  const s = getSession();
  return !!s && e.complexId === s.complexId && e.userId === s.userId;
};

const merge = (pending: AccessLogEntry[], remote: AccessLogEntry[]) => {
  const ids = new Set(pending.map((e) => e.id));
  return [...pending, ...remote.filter((e) => !ids.has(e.id))].sort((a, b) => b.at.localeCompare(a.at));
};

/** Recalcula lo derivado: nunca se muestra data de otro conjunto o persona. */
function derive(s: State): State {
  const pending = s.queue.filter(mine);
  const remote = s.remoteComplexId === getSession()?.complexId ? s.remote : [];
  return { ...s, pending, merged: merge(pending, remote) };
}

let state: State = derive({
  queue: loadPending(),
  pending: [],
  remote: [],
  remoteComplexId: null,
  loading: false,
  error: null,
  syncing: false,
  merged: [],
});

const listeners = new Set<() => void>();
function set(patch: Partial<State>) {
  state = derive({ ...state, ...patch });
  if ('queue' in patch) {
    try {
      localStorage.setItem(KEY, JSON.stringify(state.queue));
    } catch {
      /* almacenamiento lleno/bloqueado: la cola sigue en memoria */
    }
  }
  listeners.forEach((l) => l());
}

const subscribe = (cb: () => void) => {
  listeners.add(cb);
  return () => listeners.delete(cb);
};
const snapshot = () => state;

export async function refreshLog(): Promise<void> {
  const complexId = getSession()?.complexId ?? null;
  set({ loading: true });
  try {
    const rows = await api<ApiLog[]>('/access-logs?limit=500');
    set({ remote: rows.map(fromApi), remoteComplexId: complexId, error: null, loading: false });
  } catch (e) {
    set({ error: (e as Error).message, loading: false });
  }
}

/** Hook: bitácora combinada (servidor + pendientes). Refresca al montar. */
export function useAccessLog() {
  const s = useSyncExternalStore(subscribe, snapshot);
  useEffect(() => {
    void refreshLog();
  }, []);
  return { entries: s.merged, pending: s.pending, loading: s.loading, error: s.error, syncing: s.syncing, reload: refreshLog };
}

export function addLogEntry(e: Omit<AccessLogEntry, 'id' | 'at' | 'synced'>): AccessLogEntry {
  const session = getSession();
  const entry: AccessLogEntry = {
    ...e,
    id: crypto.randomUUID(),
    at: new Date().toISOString(),
    synced: false,
    complexId: session?.complexId,
    userId: session?.userId,
  };
  set({ queue: [entry, ...state.queue] });
  if (navigator.onLine) void syncPending();
  return entry;
}

let running = false;
// Pedido de sync que llegó mientras otro estaba en curso (p. ej. la salida registrada antes de
// que termine de enviarse el ingreso): se atiende al terminar, en vez de perderse.
let rerun = false;
export async function syncPending(): Promise<void> {
  if (running) {
    rerun = true;
    return;
  }
  // Siempre contra la sesión de ahora (no un derivado viejo): nunca enviar con el token de otro conjunto.
  const mineNow = state.queue.filter(mine);
  if (!mineNow.length) return;
  running = true;
  rerun = false;
  set({ syncing: true });
  try {
    // De la más antigua a la más nueva, para conservar el orden de llegada.
    for (const entry of [...mineNow].reverse()) {
      // El token de hoy solo puede registrar en su conjunto.
      try {
        await api('/access-logs', { method: 'POST', body: toApi(entry) });
      } catch (e) {
        const status = e instanceof ApiError ? e.status : 0;
        // Sin red / sesión caída / servidor caído: se conserva y se reintenta.
        if (status === 0 || status === 401 || status === 403 || status >= 500) break;
        // 4xx de validación: reintentar nunca funcionará; se descarta para no bloquear la cola.
        console.error('access-log rechazado por el servidor, se descarta', entry.id, e);
      }
      set({ queue: state.queue.filter((p) => p.id !== entry.id) });
    }
    await refreshLog();
  } finally {
    running = false;
    set({ syncing: false });
  }
  if (rerun) await syncPending();
}

if (typeof window !== 'undefined') {
  window.addEventListener('online', () => void syncPending());
}
