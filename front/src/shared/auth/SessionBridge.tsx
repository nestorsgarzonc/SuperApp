import { useEffect } from 'react';
import { useAuth as useIdentity } from '../../features/auth/components/AuthContext';
import { useAuth as useSession } from './AuthContext';
import type { Role } from '../api/client';

export function SessionBridge() {
  const { identity, activeComplex } = useIdentity();
  const { signIn, logout } = useSession();

  useEffect(() => {
    if (!identity || !activeComplex || !identity.token) { logout(); return; }
    const role = (activeComplex.role ?? 'RESIDENT') as Role;
    signIn({
      token: identity.token,
      complexId: activeComplex.id,
      complexName: activeComplex.name,
      unitId: activeComplex.unitId ?? '',
      userId: identity.id,
      role,
      name: identity.name,
      subtitle: activeComplex.name,
      memberships: identity.complexes.map((c) => ({
        complexId: c.id, complexName: c.name, role: (c.role ?? 'RESIDENT') as Role,
      })),
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [identity, activeComplex]);

  return null;
}
