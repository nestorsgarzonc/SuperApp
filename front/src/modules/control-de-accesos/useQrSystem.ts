import { useCallback, useEffect, useState } from 'react';
import { api } from '../../shared/api/client';

export interface QrSystemStatus {
  /** false cuando el circuito está abierto: usar el fallback. */
  available: boolean;
  state: 'CLOSED' | 'OPEN' | 'HALF_OPEN';
  retry_after_seconds?: number;
  fallback?: 'PRE_AUTH';
}

const UP: QrSystemStatus = { available: true, state: 'CLOSED' };

/**
 * Estado del sistema QR (lector/generador) según el circuit breaker del ms.
 * Se consulta al montar y cada `everyMs`. Si no se puede consultar, se asume
 * disponible: el 503 controlado de la propia operación avisará igualmente.
 */
export function useQrSystem(everyMs = 4000) {
  const [status, setStatus] = useState<QrSystemStatus>(UP);

  const refresh = useCallback(async () => {
    try {
      setStatus(await api<QrSystemStatus>('/access/qr-system'));
    } catch {
      /* sin información: se conserva el último estado */
    }
  }, []);

  useEffect(() => {
    void refresh();
    const t = setInterval(() => void refresh(), everyMs);
    return () => clearInterval(t);
  }, [refresh, everyMs]);

  return { ...status, refresh };
}

export type QrSystem = ReturnType<typeof useQrSystem>;
