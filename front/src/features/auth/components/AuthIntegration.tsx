import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { LoaderCircle } from 'lucide-react';
import { getAccessToken } from '../../../lib/http';
import { useAuth as useSession } from '../../../shared/auth/AuthContext';
import type { Membership, Role, Session } from '../../../shared/api/client';
import { useAuth } from './AuthContext';
import type { AuthIdentity, Complex } from '../api/authApi';

function getRole(identity: AuthIdentity, complex: Complex): Role {
  const value = (complex.role ?? identity.roles[0] ?? 'residente').toLocaleLowerCase();
  if (value.includes('admin')) return 'ADMIN';
  if (value.includes('porter') || value.includes('guard')) return 'GUARD';
  if (value.includes('propiet') || value.includes('owner')) return 'OWNER';
  return 'RESIDENT';
}

export function createSession(identity: AuthIdentity, complex: Complex): Session {
  const memberships: Membership[] = identity.complexes.map((membership) => ({
    complexId: membership.id, complexName: membership.name, role: getRole(identity, membership),
    ...(membership.unitId ? { unitId: membership.unitId } : {}),
    ...(membership.unitName ? { unitName: membership.unitName } : {}),
  }));
  const role = getRole(identity, complex);
  return {
    token: getAccessToken() ?? '', complexId: complex.id, complexName: complex.name,
    unitId: complex.unitId ?? '', ...(complex.unitName ? { unitName: complex.unitName } : {}),
    userId: identity.id, role, name: identity.name,
    subtitle: role === 'GUARD' ? 'Portería' : role === 'ADMIN' ? 'Administración' : role === 'OWNER' ? 'Propietario' : 'Residente',
    memberships,
  };
}

export function AuthSessionBridge() {
  const { identity, activeComplex } = useAuth(); const { signIn } = useSession(); const navigate = useNavigate();
  useEffect(() => { if (identity && activeComplex) { signIn(createSession(identity, activeComplex)); navigate('/', { replace: true }); } }, [identity, activeComplex, signIn, navigate]);
  return <main className="splash"><span className="spinner--large" role="status" aria-label="Cargando tu comunidad"><LoaderCircle className="loading-spinner"/></span></main>;
}

export function AuthComplexSwitcher() {
  const { identity, activeComplex, selectComplex } = useAuth(); const { session, signIn } = useSession();
  const [busy, setBusy] = useState(false);
  if (!identity || !activeComplex || !session || identity.complexes.length < 2) return <span className="chip">{activeComplex?.name}</span>;
  return <select className="chip chip-select" aria-label="Conjunto activo" value={activeComplex.id} disabled={busy} onChange={(event) => {
    const complex = identity.complexes.find(({ id }) => id === event.target.value);
    if (!complex) return;
    setBusy(true); void selectComplex(complex).then((verified) => signIn(createSession(identity, verified))).finally(() => setBusy(false));
  }}>{identity.complexes.map((complex) => <option key={complex.id} value={complex.id}>{complex.name}</option>)}</select>;
}
