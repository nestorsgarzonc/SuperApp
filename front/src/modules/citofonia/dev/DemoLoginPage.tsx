import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../../../shared/auth/AuthContext';
import { useAuth as useIdentity } from '../../../features/auth/components/AuthContext';
import { ErrorBox } from '../../../shared/ui';
import { parseIntercomClients, type IntercomClient } from '../config';
import { createDemoSession } from './demo';

export function DemoLoginPage() {
  const { signIn } = useAuth();
  const { signInDevelopment } = useIdentity();
  const navigate = useNavigate();
  const [error, setError] = useState<string | null>(null);
  let clients: IntercomClient[] = [];
  let configurationError: string | null = null;
  try {
    clients = parseIntercomClients(import.meta.env.VITE_INTERCOM_CLIENTS);
    if (!clients.length) configurationError = 'No hay perfiles configurados en VITE_INTERCOM_CLIENTS.';
  } catch (cause) {
    configurationError = cause instanceof Error ? cause.message : 'Configuración de prueba inválida.';
  }
  function enter(client: IntercomClient) {
    try {
      const session = createDemoSession(client);
      signIn(session);
      signInDevelopment(session);
      navigate('/citofonia', { replace: true });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'No se pudo crear la sesión de prueba.');
    }
  }
  return <main className="login"><div className="login-inner">
    <div className="brand">SuperApp<small>Comunicación y gestión residencial</small></div>
    <h1>Citofonía · Prueba</h1>
    <p>Elige un perfil temporal. No necesitas el backend local ni una contraseña.</p>
    <div className="alert amber"><b>Solo desarrollo</b>Las llamadas usan el WebSocket real de AWS. No hay audio ni video. Usa dos navegadores o perfiles separados, uno para cada identidad.</div>
    {clients.map((client) => <button className="profile-card" key={client.clientId} onClick={() => enter(client)}>
      <b>{client.label}</b><span>{client.role === 'porter' ? 'Portería' : 'Apartamento'} · {client.clientId}</span>
    </button>)}
    {(configurationError || error) && <ErrorBox message={configurationError ?? error!} />}
  </div></main>;
}
