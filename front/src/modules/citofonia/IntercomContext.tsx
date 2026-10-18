import { createContext, useContext, useEffect, useMemo, useSyncExternalStore, type ReactNode } from 'react';
import { useAuth } from '../../shared/auth/AuthContext';
import { resolveIntercomConfig } from './config';
import { IntercomController, type IntercomState } from './controller';
import { IntercomWebSocket } from './websocket';
import { CallNotice } from './components/CallNotice';
import './styles.css';

interface IntercomValue {
  controller: IntercomController | null;
  state: IntercomState;
  configurationError: string | null;
}
const Context = createContext<IntercomValue | null>(null);
const disconnected: IntercomState = { connection: 'disconnected', status: 'idle', call: null, delivered: false, error: null };
const emptySubscribe = () => () => {};
const emptySnapshot = () => disconnected;

export function IntercomProvider({ children }: { children: ReactNode }) {
  const { session } = useAuth();
  const userId = session?.userId;
  const complexId = session?.complexId;
  const unitId = session?.unitId;
  const role = session?.role;
  const { controller, configurationError } = useMemo(() => {
    if (!userId || !complexId || !role) return { controller: null, configurationError: null };
    try {
      const config = resolveIntercomConfig(
        { complexId, unitId: unitId ?? '', role },
        import.meta.env.VITE_INTERCOM_WS_URL,
        import.meta.env.VITE_INTERCOM_CLIENTS,
      );
      return { controller: new IntercomController(new IntercomWebSocket(), config), configurationError: null };
    } catch (error) {
      return { controller: null, configurationError: error instanceof Error ? error.message : 'Configuración de Citofonía inválida.' };
    }
  }, [userId, complexId, unitId, role]);
  const state = useSyncExternalStore(controller?.subscribe ?? emptySubscribe, controller?.getSnapshot ?? emptySnapshot);
  useEffect(() => {
    controller?.start();
    return () => controller?.stop();
  }, [controller]);
  return <Context.Provider value={{ controller, state, configurationError }}>
    {children}
    <CallNotice />
  </Context.Provider>;
}

export function useIntercom() {
  const value = useContext(Context);
  if (!value) throw new Error('useIntercom debe usarse dentro de IntercomProvider.');
  return value;
}
