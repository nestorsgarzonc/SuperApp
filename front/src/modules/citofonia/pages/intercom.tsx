import { useState, type FormEvent } from 'react';
import { Badge, ErrorBox, Field, PageHead } from '../../../shared/ui';
import { useIntercom } from '../IntercomContext';
import { CallStatus } from '../components/CallStatus';

const connectionLabels = { connected: 'Conectado', connecting: 'Conectando', disconnected: 'Desconectado' } as const;

export function IntercomPage() {
  const { controller, state, configurationError } = useIntercom();
  const [target, setTarget] = useState('');
  const config = controller?.config;
  const busy = state.status === 'calling' || state.status === 'incoming' || state.status === 'accepted';
  const selected = config?.apartments.some(({ clientId }) => clientId === target) ? target : '';
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (selected) controller?.callApartment(selected);
  }
  return <>
    <PageHead title="Citofonía" subtitle="Comunicación entre portería y apartamentos" action={
      <span role="status"><Badge tone={state.connection === 'connected' ? 'green' : state.connection === 'connecting' ? 'amber' : 'gray'}>{connectionLabels[state.connection]}</Badge></span>
    } />
    <div className="stack intercom-page">
      <div className="alert blue"><b>Prototipo de señalización</b>Permite llamar, aceptar, rechazar y finalizar. Todavía no transmite audio ni video.</div>
      {configurationError && <ErrorBox message={configurationError} />}
      {state.error && <ErrorBox message={state.error} />}
      {config && <>
        <div className="card">
          <h2>{config.client.label}</h2>
          {state.connection !== 'connected' && <p role="status">Intentando conectar con Citofonía. Las acciones estarán disponibles al recuperar la conexión.</p>}
          {config.client.role === 'porter' && <form onSubmit={submit}>
            <Field label="Apartamento">
              <select aria-label="Apartamento" value={selected} onChange={(event) => setTarget(event.target.value)} disabled={busy}>
                <option value="">Selecciona un apartamento</option>
                {config.apartments.map(({ clientId, label }) => <option key={clientId} value={clientId}>{label}</option>)}
              </select>
            </Field>
            {!config.apartments.length && <p>No hay apartamentos configurados para este conjunto.</p>}
            <div className="btn-row"><button type="submit" className="btn" disabled={!selected || busy || state.connection !== 'connected'}>Llamar</button></div>
          </form>}
          {config.client.role === 'apartment' && <div className="btn-row">
            <button className="btn" disabled={busy || state.connection !== 'connected'} onClick={() => controller.callPorter()}>Llamar a portería</button>
          </div>}
        </div>
        {state.status !== 'incoming' && <section className="card" aria-label="Llamada actual"><CallStatus /></section>}
      </>}
    </div>
  </>;
}
