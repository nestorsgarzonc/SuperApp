import { useEffect, useState, type FormEvent } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useAuth } from '../../../shared/auth/AuthContext';
import { useAsync } from '../../../shared/useAsync';
import { ApiError } from '../../../shared/api/client';
import { fmtDay, fmtLongDate, fmtRange, fmtTime, toLocalInput } from '../../../shared/format';
import { ErrorBox, Field, Loading, PageHead } from '../../../shared/ui';
import { accessApi } from '../api';
import { useQrDataUrl } from '../useQrDataUrl';
import { useQrSystem } from '../useQrSystem';
import { QrSystemNotice } from '../components/QrSystemNotice';
import { GrantBadge } from '../components/badges';
import {
  VISITOR_TYPES,
  VISITOR_TYPE_LABEL,
  grantView,
  type AccessGrant,
  type GrantView,
  type VisitorType,
} from '../types';

type Tab = GrantView | 'used' | 'history';

const TABS: { id: Tab; label: string }[] = [
  { id: 'active', label: 'Activas' },
  { id: 'upcoming', label: 'Próximas' },
  { id: 'used', label: 'Usadas' },
  { id: 'expired', label: 'Vencidas' },
  { id: 'history', label: 'Historial' },
];

// Un pase usado no es "vencido": tiene su propia pestaña.
const matchesTab = (g: AccessGrant, tab: Tab) => {
  if (tab === 'history') return true;
  if (tab === 'used') return g.status === 'USED';
  if (g.status === 'USED') return false;
  return tab === 'expired' ? ['expired', 'cancelled'].includes(grantView(g)) : grantView(g) === tab;
};

export function AuthorizationsPage() {
  const { session } = useAuth();
  const [tab, setTab] = useState<Tab>('active');
  const { data, error, loading, reload } = useAsync(
    () => (session!.unitId ? accessApi.unitGrants(session!.unitId) : Promise.resolve([])),
    [session?.unitId],
  );

  const grants = (data ?? []).slice().sort((a, b) => b.created_at.localeCompare(a.created_at));
  const count = (t: Tab) => grants.filter((g) => matchesTab(g, t)).length;
  const shown = grants.filter((g) => matchesTab(g, tab));

  if (!session!.unitId) return <MissingUnit />;

  return (
    <>
      <PageHead
        title="Autorizaciones"
        subtitle="Gestiona quién puede ingresar a tu unidad"
        action={
          <Link className="btn" to="/autorizaciones/nueva">
            + Nueva autorización
          </Link>
        }
      />
      <div className="pills" role="tablist">
        {TABS.map((t) => (
          <button
            key={t.id}
            role="tab"
            aria-selected={tab === t.id}
            className={`pill${tab === t.id ? ' active' : ''}`}
            onClick={() => setTab(t.id)}
          >
            {t.label}
            {t.id !== 'history' && ` (${count(t.id)})`}
          </button>
        ))}
      </div>

      {loading && <Loading />}
      {error && <ErrorBox message={error} onRetry={reload} />}
      {!loading && !error && (
        <div className="card">
          {shown.length === 0 ? (
            <div className="empty">No hay autorizaciones en esta categoría.</div>
          ) : (
            <ul className="list">
              {shown.map((g) => (
                <li key={g.id}>
                  <Link to={`/autorizaciones/${g.id}`} style={{ flex: 1, minWidth: 0 }}>
                    <div className="t">{g.visitor_name}</div>
                    <div className="s">
                      {VISITOR_TYPE_LABEL[g.visitor_type]}
                      {g.method === 'PRE_AUTH' && ' · Por documento'} · {fmtRange(g.valid_from ?? g.created_at, g.valid_until)}
                      {vehicleOf(g) && ` · ${vehicleOf(g)}`}
                    </div>
                  </Link>
                  <GrantBadge grant={g} />
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </>
  );
}

function MissingUnit() {
  return (
    <div className="alert amber">
      <b>Sin unidad asignada</b>
      Tu cuenta aún no está asociada a una unidad de este conjunto. Contacta a la administración.
    </div>
  );
}

/** El diseño pide vehículo/placa; el backend lo guarda en `details`. */
function vehicleOf(g: AccessGrant) {
  const m = g.details?.match(/Placa:\s*(\S+)/i);
  return m ? m[1] : '';
}

const combine = (day: string, time: string) => new Date(`${day}T${time}`).toISOString();

const hm = (d: Date) => `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
/** Por defecto el pase empieza ahora y dura 4 horas (sin pasar de la medianoche). */
const defaultWindow = () => {
  const now = new Date();
  const end = new Date(now.getTime() + 4 * 3600e3);
  return { start: hm(now), end: end.getDate() === now.getDate() ? hm(end) : '23:59' };
};

export function NewAuthorizationPage() {
  const { session } = useAuth();
  const nav = useNavigate();
  const [name, setName] = useState('');
  const [day, setDay] = useState(toLocalInput(new Date()));
  const [type, setType] = useState<VisitorType>('VISITOR');
  const [window0] = useState(defaultWindow);
  const [start, setStart] = useState(window0.start);
  const [end, setEnd] = useState(window0.end);
  const [vehicle, setVehicle] = useState('');
  const [plate, setPlate] = useState('');
  const [method, setMethod] = useState<'QR' | 'PRE_AUTH'>('QR');
  const qr = useQrSystem();
  // Circuit breaker abierto: el QR no se puede emitir, se pasa a pre-autorización por documento.
  useEffect(() => {
    if (!qr.available) setMethod('PRE_AUTH');
  }, [qr.available]);
  const [doc, setDoc] = useState('');
  const [busy, setBusy] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [apiError, setApiError] = useState<string | null>(null);

  if (!session!.unitId) return <MissingUnit />;

  async function submit(e: FormEvent) {
    e.preventDefault();
    const errs: Record<string, string> = {};
    if (!name.trim()) errs.name = 'Escribe el nombre del visitante';
    if (method === 'PRE_AUTH' && !doc.trim()) errs.doc = 'El guardia validará con este documento';
    if (end <= start) errs.end = 'Debe ser posterior a la hora de inicio';
    else if (new Date(`${day}T${end}`).getTime() < Date.now()) errs.end = 'La hora fin ya pasó';
    setErrors(errs);
    if (Object.keys(errs).length) return;

    setBusy(true);
    setApiError(null);
    const details = [vehicle && `Vehículo: ${vehicle}`, plate && `Placa: ${plate.toUpperCase()}`]
      .filter(Boolean)
      .join(' · ');
    try {
      const g = await accessApi.createGrant({
        unit_id: session!.unitId,
        method,
        visitor_type: type,
        visitor_name: name.trim(),
        visitor_id_number: method === 'PRE_AUTH' ? doc.trim() : undefined,
        details: details || undefined,
        valid_from: combine(day, start),
        valid_until: combine(day, end),
      });
      nav(`/autorizaciones/${g.id}`, { replace: true });
    } catch (err) {
      if (err instanceof ApiError && err.code === 'QR_001') {
        setMethod('PRE_AUTH');
        void qr.refresh();
        setApiError('El sistema de QR no está disponible. Cambiamos a "Por documento": escribe el documento del visitante y vuelve a intentar.');
        setBusy(false);
        return;
      }
      const details = err instanceof ApiError ? err.details : undefined;
      setApiError(
        err instanceof Error ? err.message + (details ? ` (${JSON.stringify(details)})` : '') : 'Error desconocido',
      );
      setBusy(false);
    }
  }

  return (
    <>
      <PageHead title="Nueva autorización" subtitle={session!.complexName} />
      <form className="card form" onSubmit={submit} noValidate>
        <div className="field full">
          <label>Cómo se identificará</label>
          <div className="pills" role="radiogroup" style={{ marginBottom: 0 }}>
            <button type="button" role="radio" aria-checked={method === 'QR'} className={`pill${method === 'QR' ? ' active' : ''}`} onClick={() => setMethod('QR')} disabled={!qr.available} title={qr.available ? undefined : 'Sistema de QR no disponible'}>
              Pase con QR
            </button>
            <button type="button" role="radio" aria-checked={method === 'PRE_AUTH'} className={`pill${method === 'PRE_AUTH' ? ' active' : ''}`} onClick={() => setMethod('PRE_AUTH')}>
              Por documento
            </button>
          </div>
        </div>
        {!qr.available && (
          <div className="field full">
            <QrSystemNotice qr={qr} audience="resident" />
          </div>
        )}
        <Field label="Visitante (nombre completo)" full error={errors.name}>
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Juan Pérez" maxLength={200} />
        </Field>
        {method === 'PRE_AUTH' && (
          <Field label="Documento del visitante" full error={errors.doc}>
            <input value={doc} onChange={(e) => setDoc(e.target.value)} placeholder="1.020.345.678" inputMode="numeric" maxLength={50} />
          </Field>
        )}
        <Field label="Fecha">
          <input type="date" value={day} min={toLocalInput(new Date())} onChange={(e) => setDay(e.target.value)} />
        </Field>
        <Field label="Tipo de acceso">
          <select value={type} onChange={(e) => setType(e.target.value as VisitorType)}>
            {VISITOR_TYPES.map((t) => (
              <option key={t} value={t}>
                {VISITOR_TYPE_LABEL[t]}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Hora inicio">
          <input type="time" value={start} onChange={(e) => setStart(e.target.value)} />
        </Field>
        <Field label="Hora fin" error={errors.end}>
          <input type="time" value={end} onChange={(e) => setEnd(e.target.value)} />
        </Field>
        <Field label="Vehículo (opcional)">
          <input value={vehicle} onChange={(e) => setVehicle(e.target.value)} placeholder="Automóvil" />
        </Field>
        <Field label="Placa (opcional)">
          <input value={plate} onChange={(e) => setPlate(e.target.value)} placeholder="ABC123" maxLength={10} />
        </Field>
        {new Date(`${day}T${start}`).getTime() > Date.now() + 60_000 && (
          <div className="field full">
            <div className="alert blue">
              El pase <b style={{ display: 'inline' }}>aún no estará vigente</b> hasta las {fmtTime(combine(day, start))}. Si el visitante llega antes, portería lo rechazará.
            </div>
          </div>
        )}
        {apiError && (
          <div className="field full">
            <ErrorBox message={apiError} />
          </div>
        )}
        <div className="field full">
          <button className="btn block" disabled={busy}>
            {busy ? 'Generando…' : method === 'QR' ? 'Generar pase' : 'Autorizar visitante'}
          </button>
          <button type="button" className="btn ghost block" onClick={() => nav(-1)}>
            Cancelar
          </button>
        </div>
      </form>
    </>
  );
}

export function AccessPassPage() {
  const { session } = useAuth();
  const { id } = useParams();
  const nav = useNavigate();
  const { data: g, error, loading, reload } = useAsync(() => accessApi.getGrant(id!), [id]);
  const qr = useQrDataUrl(g?.qr_code_hash);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);

  async function revoke() {
    setBusy(true);
    setActionError(null);
    try {
      await accessApi.cancelGrant(id!);
      setConfirming(false);
      reload();
    } catch (e) {
      setActionError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function share() {
    if (!g) return;
    const text = `Pase de acceso SuperApp — ${g.visitor_name}\n${g.method === 'PRE_AUTH' ? `Documento: ${g.visitor_id_number}` : `Código: ${g.qr_code_hash ?? g.id}`}`;
    try {
      if (navigator.share) await navigator.share({ title: 'Pase de acceso', text });
      else {
        await navigator.clipboard.writeText(text);
        setNotice('Datos del pase copiados al portapapeles');
      }
    } catch {
      /* el usuario canceló */
    }
  }

  function download() {
    if (!g || !qr) return;
    const a = document.createElement('a');
    a.href = qr;
    a.download = `pase-${g.visitor_name.replace(/\s+/g, '-')}.png`;
    a.click();
  }

  if (loading) return <Loading />;
  if (error || !g) return <ErrorBox message={error ?? 'Pase no encontrado'} onRetry={reload} />;
  const active = grantView(g) === 'active' || grantView(g) === 'upcoming';
  const isPreAuth = g.method === 'PRE_AUTH';

  return (
    <>
      <PageHead
        title="Pase de acceso"
        subtitle={isPreAuth ? 'El visitante se identifica con su documento en portería' : 'El código es válido por 12 horas desde su generación'}
      />
      <div className="card pass">
        <div className="pass-qr">
          {isPreAuth ? (
            <div style={{ textAlign: 'center' }}>
              <div className="s" style={{ color: 'var(--muted)' }}>Documento</div>
              <b style={{ fontSize: 18, wordBreak: 'break-all' }}>{g.visitor_id_number}</b>
            </div>
          ) : qr ? (
            <img src={qr} alt={`Código QR de ${g.visitor_name}`} />
          ) : (
            <span>{g.qr_code_hash ? 'Generando…' : 'Sin QR'}</span>
          )}
        </div>
        <div>
          <GrantBadge grant={g} />
          <h2 style={{ fontSize: 22, margin: '8px 0 2px' }}>{g.visitor_name}</h2>
          <div className="s" style={{ color: 'var(--muted)' }}>
            {VISITOR_TYPE_LABEL[g.visitor_type]}
            {g.unit_name && ` · ${g.unit_name}`}
            {g.creator_name && ` · Residente: ${g.creator_name}`}
          </div>
          <div className="kv">
            <small>Conjunto</small>{session!.complexName}
          </div>
          <div className="kv">
            <b>
              {g.valid_until && `${fmtLongDate(g.valid_until)} · `}
              {fmtTime(g.valid_from ?? g.created_at)} – {fmtTime(g.valid_until)}
            </b>
          </div>
          {g.details && <div className="s" style={{ color: 'var(--muted)' }}>{g.details}</div>}
          <div className="kv">
            <b className="code" style={{ color: 'var(--primary)' }}>
              {isPreAuth ? `Documento: ${g.visitor_id_number}` : `Código: ${g.qr_code_hash ?? g.id}`}
            </b>
          </div>
        </div>
      </div>

      {grantView(g) === 'upcoming' && g.valid_from && (
        <div className="alert blue" style={{ marginTop: 14 }}>
          <b>Este pase aún no está vigente</b>
          Empieza {fmtDay(g.valid_from)} a las {fmtTime(g.valid_from)}. Antes de esa hora la portería lo rechazará.
        </div>
      )}
      {notice && <div className="alert green" style={{ marginTop: 14 }}>{notice}</div>}
      {actionError && <div style={{ marginTop: 14 }}><ErrorBox message={actionError} /></div>}

      <div className="btn-row">
        <button className="btn" onClick={share} disabled={!active}>
          Compartir pase
        </button>
        {!isPreAuth && (
          <button className="btn secondary" onClick={download} disabled={!qr}>
            Descargar
          </button>
        )}
        {active && !confirming && (
          <button className="btn danger" onClick={() => setConfirming(true)}>
            Revocar
          </button>
        )}
      </div>
      {confirming && (
        <div className="alert red" style={{ marginTop: 14 }}>
          <b>¿Revocar este pase?</b>
          El visitante ya no podrá usarlo para ingresar.
          <div className="btn-row">
            <button className="btn danger" onClick={revoke} disabled={busy}>
              {busy ? 'Revocando…' : 'Sí, revocar'}
            </button>
            <button className="btn outline" onClick={() => setConfirming(false)}>
              Cancelar
            </button>
          </div>
        </div>
      )}
      <div className="btn-row">
        <button className="btn ghost" onClick={() => nav('/autorizaciones')}>
          ← Volver a autorizaciones
        </button>
      </div>
    </>
  );
}
