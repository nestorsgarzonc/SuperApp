import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../../../shared/auth/AuthContext';
import { useAuth as useIdentity } from '../../../features/auth/components/AuthContext';
import type { Session } from '../../../shared/api/client';
import { ErrorBox } from '../../../shared/ui';
import { listDevUsers, loginDemo, membershipLabel, type DevUser } from './devApi';

/**
 * Entrada de PRUEBAS de control-de-accesos (ver devApi.ts). No es el login de
 * la app: solo existe para poder probar el módulo con distintas personas y
 * conjuntos hasta que el módulo `auth` aporte el login real.
 */
export function DevLoginPage() {
  const { signIn } = useAuth();
  const { signInDevelopment } = useIdentity();
  const navigate = useNavigate();
  const [users, setUsers] = useState<DevUser[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  // Persona con varios conjuntos esperando que elija uno.
  const [choosing, setChoosing] = useState<DevUser | null>(null);

  useEffect(() => {
    if (!import.meta.env.DEV) return;
    listDevUsers()
      .then(setUsers)
      .catch((e: Error) => setLoadError(e.message));
  }, []);

  async function run(key: string, fn: () => Promise<Session>) {
    setBusy(key);
    setError(null);
    try {
      const session = await fn();
      signIn(session);
      signInDevelopment(session);
      navigate('/', { replace: true });
    } catch (e) {
      setError((e as Error).message);
      setBusy(null);
    }
  }

  if (!import.meta.env.DEV) {
    return (
      <div className="login">
        <div className="login-inner">
          <h1>Iniciar sesión</h1>
          <p>El inicio de sesión lo aporta el módulo de autenticación (pendiente de integrar).</p>
        </div>
      </div>
    );
  }

  if (choosing) {
    return (
      <div className="login">
        <div className="login-inner">
          <h1>Selecciona tu conjunto</h1>
          <p>{choosing.name}, tienes accesos en más de un conjunto. Elige con cuál quieres trabajar.</p>
          {choosing.memberships.map((m) => {
            const key = `${m.complexId}:${m.role}:${m.unitId ?? ''}`;
            return (
              <button
                key={key}
                className="profile-card"
                disabled={busy !== null}
                onClick={() => void run(key, () => loginDemo(choosing.userId, m))}
              >
                <b>{m.complexName}</b>
                <span>{busy === key ? 'Ingresando…' : membershipLabel(m)}</span>
              </button>
            );
          })}
          {error && <ErrorBox message={error} />}
          <div className="btn-row" style={{ justifyContent: 'center' }}>
            <button className="btn ghost" onClick={() => setChoosing(null)}>
              ← Volver
            </button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="login">
      <div className="login-inner" data-testid="dev-login">
        <div className="brand" style={{ marginBottom: 18 }}>
          SuperApp<small>Comunicación y gestión residencial</small>
        </div>
        <h1>Control de acceso · pruebas</h1>
        <p>Entrada de desarrollo: elige una persona de la base. El login real lo aportará el módulo de autenticación.</p>
        {loadError && <ErrorBox message={`${loadError}. ¿Está corriendo el ms de control-de-accesos con ENV=development?`} />}
        {users?.map((u) => (
          <button
            key={u.userId}
            className="profile-card"
            disabled={busy !== null}
            onClick={() => {
              const membership = u.memberships[0];
              if (u.memberships.length === 1 && membership) void run(u.userId, () => loginDemo(u.userId, membership));
              else setChoosing(u);
            }}
          >
            <b>{u.name}</b>
            <span>
              {busy === u.userId
                ? 'Ingresando…'
                : u.memberships.length === 0
                  ? 'Sin conjuntos'
                  : u.memberships.map((m) => `${m.complexName} (${membershipLabel(m)})`).join(' · ')}
            </span>
          </button>
        ))}
        {error && <ErrorBox message={error} />}
      </div>
    </div>
  );
}
