import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';
import { setApiSession, type Session } from '../api/client';

interface AuthValue {
  session: Session | null;
  profile: { role: Session['role']; name: string; subtitle: string } | null;
  /** Lo llama quien autentica (módulo auth, o la entrada de pruebas de accesos). */
  signIn: (s: Session) => void;
  logout: () => void;
}

const STORAGE_KEY = 'superapp.session';
const AuthContext = createContext<AuthValue | null>(null);

function loadStored(): Session | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? (JSON.parse(raw) as Session) : null;
  } catch {
    return null;
  }
}

const persist = (s: Session | null) => {
  try {
    if (s) localStorage.setItem(STORAGE_KEY, JSON.stringify(s));
    else localStorage.removeItem(STORAGE_KEY);
  } catch {
    /* almacenamiento bloqueado: la sesión vive solo en memoria */
  }
};

const isExpired = (token: string) => {
  try {
    const { exp } = JSON.parse(atob(token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/')));
    return typeof exp === 'number' && exp * 1000 < Date.now();
  } catch {
    return true;
  }
};

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(() => {
    const s = loadStored();
    const usable = s && !isExpired(s.token) ? s : null;
    if (!usable) persist(null);
    setApiSession(usable);
    return usable;
  });

  const signIn = useCallback((s: Session) => {
    setApiSession(s);
    persist(s);
    setSession(s);
  }, []);

  const logout = useCallback(() => {
    setApiSession(null);
    persist(null);
    setSession(null);
  }, []);

  const value = useMemo<AuthValue>(
    () => ({
      session,
      profile: session ? { role: session.role, name: session.name, subtitle: session.subtitle } : null,
      signIn,
      logout,
    }),
    [session, signIn, logout],
  );
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth fuera de AuthProvider');
  return ctx;
}
