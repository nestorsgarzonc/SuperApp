import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../../../shared/auth/AuthContext';
import { useAuth as useIdentity } from '../../../features/auth/components/AuthContext';
import type { Membership } from '../../../shared/api/client';
import { ROLE_LABEL, loginDemo } from './devApi';

const keyOf = (m: Membership) => `${m.complexId}:${m.role}:${m.unitId ?? ''}`;

/**
 * SOLO PRUEBAS (ver devApi.ts). Chip del conjunto activo; si la persona
 * pertenece a varios conjuntos (o tiene roles distintos en cada uno) se vuelve
 * un selector: al cambiar se pide un token nuevo para ese conjunto, así cada
 * sesión opera en un único conjunto.
 */
export function ComplexSwitcher() {
  const { session, signIn } = useAuth();
  const { signInDevelopment } = useIdentity();
  const nav = useNavigate();
  const [busy, setBusy] = useState(false);
  if (!session) return null;

  const options = session.memberships ?? [];
  if (options.length < 2) return <span className="chip">{session.complexName}</span>;

  const current = options.find(
    (m) => m.complexId === session.complexId && m.role === session.role && (m.unitId ?? '') === session.unitId,
  );

  async function change(key: string) {
    const m = options.find((o) => keyOf(o) === key);
    if (!m || !session) return;
    setBusy(true);
    try {
      const next = await loginDemo(session.userId, m);
      signIn(next);
      signInDevelopment(next);
      nav('/', { replace: true });
    } finally {
      setBusy(false);
    }
  }

  return (
    <select
      className="chip chip-select"
      aria-label="Conjunto activo"
      value={current ? keyOf(current) : ''}
      disabled={busy}
      onChange={(e) => void change(e.target.value)}
    >
      {options.map((m) => (
        <option key={keyOf(m)} value={keyOf(m)}>
          {m.complexName} · {ROLE_LABEL[m.role]}
          {m.unitName ? ` ${m.unitName}` : ''}
        </option>
      ))}
    </select>
  );
}
