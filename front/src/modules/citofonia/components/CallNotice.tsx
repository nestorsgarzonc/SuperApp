import { useEffect, useRef } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { ErrorBox } from '../../../shared/ui';
import { useIntercom } from '../IntercomContext';
import { CallStatus } from './CallStatus';

export function CallNotice() {
  const { state, controller } = useIntercom();
  const { pathname } = useLocation();
  const dialog = useRef<HTMLDialogElement>(null);
  const incoming = state.status === 'incoming';
  useEffect(() => {
    const element = dialog.current;
    if (incoming) element?.showModal();
    return () => element?.close();
  }, [incoming]);
  if (incoming) return <dialog ref={dialog} className="intercom-dialog card" aria-label="Llamada entrante de portería" onCancel={(event) => {
    event.preventDefault();
    controller?.reject();
  }}>
    <CallStatus />
    {state.error && <ErrorBox message={state.error} />}
  </dialog>;
  if (pathname === '/citofonia' || (state.status !== 'calling' && state.status !== 'accepted' && !state.error)) return null;
  return <aside className="intercom-notice card" aria-label="Estado de Citofonía">
    <Link className="btn secondary" to="/citofonia">Abrir Citofonía</Link>
    {(state.status === 'calling' || state.status === 'accepted') && <CallStatus />}
    {state.error && <ErrorBox message={state.error} />}
  </aside>;
}
