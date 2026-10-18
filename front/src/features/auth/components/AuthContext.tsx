import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import { activateComplex, login as loginRequest, logout as logoutRequest, type AuthIdentity, type Complex } from '../api/authApi';
import { setAccessToken } from '../../../lib/http';
import type { Membership, Role, Session } from '../../../shared/api/client';

interface AuthContextValue {
  identity: AuthIdentity | null; activeComplex: Complex | null; loading: boolean;
  signIn: (email: string, password: string, remember: boolean) => Promise<void>;
  signInDevelopment: (session: Session) => void;
  selectComplex: (complex: Complex) => Promise<Complex>; signOut: (all?: boolean) => Promise<void>;
}
const AuthContext = createContext<AuthContextValue | null>(null);

const permissionsForRole: Record<Role, string[]> = {
  RESIDENT: ['access.home.read', 'access.authorizations.read', 'access.authorizations.create', 'access.building.read'],
  OWNER: ['access.home.read', 'access.authorizations.read', 'access.authorizations.create', 'access.building.read'],
  GUARD: ['access.home.read', 'access.pass.validate', 'access.visitor.register', 'access.movement.register', 'access.history.read'],
  ADMIN: ['access.home.read', 'access.authorizations.read', 'access.authorizations.create', 'access.building.read', 'access.pass.validate', 'access.visitor.register', 'access.movement.register', 'access.history.read'],
};

function toDevComplex(membership: Membership): Complex {
  return {
    id: membership.complexId,
    name: membership.complexName,
    address: '',
    status: 'Activo',
    role: membership.role,
    permissions: permissionsForRole[membership.role],
    ...(membership.unitId ? { unitId: membership.unitId } : {}),
    ...(membership.unitName ? { unitName: membership.unitName } : {}),
  };
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [identity, setIdentity] = useState<AuthIdentity | null>(null);
  const [activeComplex, setActiveComplex] = useState<Complex | null>(null);
  const [loading, setLoading] = useState(false);
  const [developmentSession, setDevelopmentSession] = useState(false);
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
      setDevelopmentSession(false);
      if (authenticated.complexes.length > 1) {
        setIdentity(authenticated);
        navigate('/select-complex');
      } else {
        const only = authenticated.complexes[0]!;
        setActiveComplex(only);
        setIdentity({ ...authenticated, permissions: only.permissions ?? [], activeComplex: only });
        navigate('/dashboard');
      }
    } finally { setLoading(false); }
  }, [navigate]);
  const signInDevelopment = useCallback((session: Session) => {
    const complexes = (session.memberships ?? []).map(toDevComplex);
    const active = complexes.find(({ id }) => id === session.complexId) ?? toDevComplex({
      complexId: session.complexId,
      complexName: session.complexName,
      role: session.role,
      ...(session.unitId ? { unitId: session.unitId } : {}),
      ...(session.unitName ? { unitName: session.unitName } : {}),
    });
    const identity: AuthIdentity = {
      id: session.userId,
      name: session.name,
      email: `${session.userId}@dev.local`,
      roles: [session.role],
      permissions: active.permissions ?? [],
      complexes: complexes.length > 0 ? complexes : [active],
      activeComplex: active,
    };
    setDevelopmentSession(true);
    setIdentity(identity);
    setActiveComplex(active);
    navigate('/', { replace: true });
  }, [navigate]);
  const selectComplex = useCallback(async (complex: Complex) => {
    if (!identity?.complexes.some(({ id }) => id === complex.id)) throw new Error('El conjunto no pertenece a la identidad autenticada.');
    const authenticatedComplex = await activateComplex(complex);
    setActiveComplex(authenticatedComplex);
    setIdentity((current) => current ? { ...current, activeComplex: authenticatedComplex, permissions: authenticatedComplex.permissions ?? current.permissions } : current);
    return authenticatedComplex;
  }, [identity]);
  const signOut = useCallback(async (all = false) => {
    try { if (!developmentSession) await logoutRequest(all); } finally {
      setIdentity(null); setActiveComplex(null); setAccessToken(null); navigate('/login', { replace: true });
    }
  }, [developmentSession, navigate]);
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
  const value = useMemo(() => ({ identity, activeComplex, loading, signIn, signInDevelopment, selectComplex, signOut }), [identity, activeComplex, loading, signIn, signInDevelopment, selectComplex, signOut]);
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) throw new Error('useAuth debe usarse dentro de AuthProvider.');
  return context;
}
