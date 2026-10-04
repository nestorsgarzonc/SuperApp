import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { setApiSession, type Session } from '../api/client';

interface AuthValue {
  session: Session | null;
  profile: { role: Session['role']; name: string; subtitle: string } | null;
  /** Lo llama quien autentica (módulo auth, o la entrada de pruebas de accesos). */
  signIn: (s: Session) => void;
  logout: () => void;
}

const AuthContext = createContext<AuthValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(() => { setApiSession(null); return null; });

  const signIn = useCallback((s: Session) => {
    setApiSession(s);
    setSession(s);
  }, []);

  const logout = useCallback(() => {
    setApiSession(null);
    setSession(null);
  }, []);
  useEffect(() => {
    const clear = () => logout();
    window.addEventListener('superapp:session-expired', clear);
    return () => window.removeEventListener('superapp:session-expired', clear);
  }, [logout]);

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
