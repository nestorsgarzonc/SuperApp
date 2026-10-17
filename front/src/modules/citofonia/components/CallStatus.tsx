import { Phone, PhoneOff } from 'lucide-react';
import { useIntercom } from '../IntercomContext';

export function CallStatus() {
  const { controller, state } = useIntercom();
  const { call, status, connection } = state;
  const enabled = connection === 'connected';
  return <div className="intercom-call" aria-live="polite">
    <span className="intercom-call__icon" aria-hidden="true">{status === 'rejected' || status === 'ended' ? <PhoneOff /> : <Phone />}</span>
    {status === 'idle' && <><h2>Sin llamadas en curso</h2><p>{controller?.config.client.role === 'porter' ? 'Selecciona un apartamento para llamar.' : 'Recibirás aquí las llamadas de portería mientras tu sesión esté abierta.'}</p></>}
    {status === 'calling' && <><h2>Llamando {call?.label}...</h2><p>Esperando respuesta...</p>{state.delivered && <p>Solicitud entregada al apartamento.</p>}</>}
    {status === 'incoming' && <><h2>Portería está llamando</h2><p>Llamada entrante para {controller?.config.client.label}.</p></>}
    {status === 'accepted' && <><h2>Llamada aceptada</h2><p>{call?.label} · Señalización establecida.</p><p>Este prototipo no transmite audio ni video.</p></>}
    {status === 'rejected' && <><h2>Llamada rechazada</h2><p>{call?.direction === 'outgoing' ? 'El apartamento rechazó la llamada.' : 'Rechazaste la llamada de portería.'}</p></>}
    {status === 'ended' && <><h2>Llamada finalizada</h2><p>Ya no hay una llamada en curso.</p></>}
    {status === 'error' && <h2>No se pudo continuar la llamada</h2>}
    {status === 'incoming' && <div className="btn-row">
      <button className="btn" disabled={!enabled} onClick={() => controller?.accept()}>Aceptar</button>
      <button className="btn danger-soft" disabled={!enabled} onClick={() => controller?.reject()}>Rechazar</button>
    </div>}
    {(status === 'calling' || status === 'accepted') && <div className="btn-row">
      <button className="btn danger" disabled={!enabled} onClick={() => controller?.hangup()}>{status === 'calling' ? 'Cancelar llamada' : 'Finalizar llamada'}</button>
    </div>}
  </div>;
}
