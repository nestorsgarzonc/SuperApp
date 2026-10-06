import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import { activateComplex, login as loginRequest, logout as logoutRequest, type AuthIdentity, type Complex } from '../api/authApi';
import { setAccessToken } from '../../../lib/http';

interface AuthContextValue {
  identity: AuthIdentity | null; activeComplex: Complex | null; loading: boolean;
  signIn: (email: string, password: string, remember: boolean) => Promise<void>;
  selectComplex: (complex: Complex) => Promise<Complex>; signOut: (all?: boolean) => Promise<void>;
}
const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [identity, setIdentity] = useState<AuthIdentity | null>(null);
  const [activeComplex, setActiveComplex] = useState<Complex | null>(null);
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();
  const signIn = useCallback(async (email: string, password: string, remember: boolean) => {
    setLoading(true);
    try {
      const authenticated = await loginRequest(email, password, remember);
      if (authenticated.complexes.length === 0) {
        setAccessToken(null);
        setIdentity(null);
        navigate('/unauthorized', { replace: true });
        return;
      }
      setIdentity(authenticated);
      if (authenticated.complexes.length > 1) navigate('/select-complex');
      else { setActiveComplex(authenticated.complexes[0] ?? null); navigate('/dashboard'); }
    } finally { setLoading(false); }
  }, [navigate]);
  const selectComplex = useCallback(async (complex: Complex) => {
    if (!identity?.complexes.some(({ id }) => id === complex.id)) throw new Error('El conjunto no pertenece a la identidad autenticada.');
    const authenticatedComplex = await activateComplex(complex);
    setActiveComplex(authenticatedComplex);
    setIdentity((current) => current ? { ...current, activeComplex: authenticatedComplex, permissions: authenticatedComplex.permissions ?? current.permissions } : current);
    return authenticatedComplex;
  }, [identity]);
  const signOut = useCallback(async (all = false) => {
    try { await logoutRequest(all); } finally {
      setIdentity(null); setActiveComplex(null); setAccessToken(null); navigate('/login', { replace: true });
    }
  }, [navigate]);
  useEffect(() => {
    const expire = () => { setIdentity(null); setActiveComplex(null); navigate('/session-expired', { replace: true }); };
    const refreshed = (event: Event) => {
      const token = (event as CustomEvent<{ token?: string }>).detail?.token;
      if (token) setAccessToken(token);
    };
    window.addEventListener('superapp:session-expired', expire);
    window.addEventListener('superapp:access-token-refreshed', refreshed);
    return () => { window.removeEventListener('superapp:session-expired', expire); window.removeEventListener('superapp:access-token-refreshed', refreshed); };
  }, [navigate]);
  const value = useMemo(() => ({ identity, activeComplex, loading, signIn, selectComplex, signOut }), [identity, activeComplex, loading, signIn, selectComplex, signOut]);
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) throw new Error('useAuth debe usarse dentro de AuthProvider.');
  return context;
}
