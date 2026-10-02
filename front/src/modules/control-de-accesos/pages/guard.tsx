import { useState, type FormEvent } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '../../../shared/auth/AuthContext';
import { ApiError } from '../../../shared/api/client';
import { fmtDay, fmtTime } from '../../../shared/format';
import { ErrorBox, Field, PageHead } from '../../../shared/ui';
import { accessApi } from '../api';
import { useQrSystem } from '../useQrSystem';
import { QrSystemNotice } from '../components/QrSystemNotice';
import { addLogEntry, type Movement } from '../accessLog';
import { unitLabel } from '../unit';
import { VISITOR_TYPES, VISITOR_TYPE_LABEL, rejectText, type ValidateResult, type VisitorType } from '../types';

const when = (iso: string) => `${fmtDay(iso)} a las ${fmtTime(iso)}`;

interface MovementState {
  visitor: string;
  unit: string;
  visitorType: VisitorType;
  vehicle?: string;
  grantId?: string;
}

type ValidateMode = 'QR' | 'PRE_AUTH';

export function ValidatePassPage() {
  const { session, profile } = useAuth();
  const nav = useNavigate();
  const [mode, setMode] = useState<ValidateMode>('QR');
  const qr = useQrSystem();
  const [code, setCode] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<ValidateResult | null>(null);

  async function validate(e: FormEvent) {
    e.preventDefault();
    if (!code.trim()) {
      return setError(mode === 'QR' ? 'Escribe o escanea el código del pase' : 'Escribe el número de documento del visitante');
    }
    setBusy(true);
    setError(null);
    setResult(null);
    try {
      const r =
        mode === 'QR'
          ? await accessApi.validateQR(session!.complexId, code)
          : await accessApi.validatePreAuth(session!.complexId, code);
      setResult(r);
      if (!r.valid) {
        addLogEntry({
          movement: 'ATTEMPT',
          result: 'DENIED',
          visitor: r.visitor_name ?? 'Sin identificar',
          unit: unitLabel(r.unit_id, r.unit_name),
          visitorType: r.visitor_type ? VISITOR_TYPE_LABEL[r.visitor_type] : '—',
          registeredBy: profile!.name,
        });
      }
    } catch (err) {
      if (err instanceof ApiError && err.code === 'QR_001') {
        void qr.refresh();
        setMode('PRE_AUTH');
        setCode('');
        setError('El sistema de QR no responde. Pasamos a validar por documento (pre-autorización).');
      } else {
        setError((err as Error).message);
      }
    } finally {
      setBusy(false);
    }
  }

  function switchMode(next: ValidateMode) {
    setMode(next);
    setCode('');
    setError(null);
    setResult(null);
  }

  const g = result?.access_grant;
  const toMovement = () =>
    nav('/porteria/movimiento', {
      state: {
        visitor: result?.visitor_name ?? g?.visitor_name ?? '',
        unit: unitLabel(result?.unit_id ?? g?.unit_id, result?.unit_name ?? g?.unit_name),
        visitorType: (result?.visitor_type ?? g?.visitor_type ?? 'VISITOR') as VisitorType,
        vehicle: g?.details,
        grantId: g?.id,
      } satisfies MovementState,
    });

  return (
    <>
      <PageHead title="Validar pase" subtitle="Escanea el QR, escribe el código o valida por documento" />
      <div style={{ marginBottom: 14 }}>
        <QrSystemNotice qr={qr} audience="guard">
          {mode === 'QR' && (
            <div className="btn-row">
              <button className="btn" onClick={() => switchMode('PRE_AUTH')}>
                Validar por documento
              </button>
            </div>
          )}
        </QrSystemNotice>
      </div>
      <div className="pills" role="tablist" aria-label="Método de validación">
        <button role="tab" aria-selected={mode === 'QR'} className={`pill${mode === 'QR' ? ' active' : ''}`} onClick={() => switchMode('QR')}>
          Código QR
        </button>
        <button
          role="tab"
          aria-selected={mode === 'PRE_AUTH'}
          className={`pill${mode === 'PRE_AUTH' ? ' active' : ''}`}
          onClick={() => switchMode('PRE_AUTH')}
        >
          Documento (pre-autorización)
        </button>
      </div>
      <div className="two-col">
        <form className="card" onSubmit={validate}>
          {mode === 'QR' ? (
            <>
              <div className="scan-box">📷 Escanear QR</div>
              <Field label="O ingresa el código">
                <input value={code} onChange={(e) => setCode(e.target.value)} placeholder="SA-7K29-4QX1" autoCapitalize="off" />
              </Field>
            </>
          ) : (
            <Field label="Documento del visitante">
              <input value={code} onChange={(e) => setCode(e.target.value)} placeholder="1.020.345.678" inputMode="numeric" maxLength={50} />
            </Field>
          )}
          <div className="btn-row" style={{ justifyContent: 'center' }}>
            <button className="btn" disabled={busy || (mode === 'QR' && !qr.available)}>
              {busy ? 'Validando…' : 'Validar pase'}
            </button>
          </div>
        </form>

        <div>
          {error && <ErrorBox message={error} />}
          {result?.valid && (
            <div className="result-ok">
              <h2>✓ Pase válido</h2>
              <div className="kv">
                <small>Visitante</small>
                <b>{result.visitor_name ?? g?.visitor_name}</b>
              </div>
              <div className="kv">
                <small>Unidad</small>
                <b>{unitLabel(result.unit_id ?? g?.unit_id, result.unit_name ?? g?.unit_name)}</b>
              </div>
              {g && (
                <div className="kv">
                  <small>Vigencia</small>
                  <b>
                    {fmtTime(g.valid_from ?? g.created_at)} – {fmtTime(g.valid_until)}
                  </b>
                </div>
              )}
              <p style={{ fontSize: 13, marginBottom: 10 }}>{result.message}</p>
              <button className="btn" onClick={toMovement}>
                Registrar ingreso
              </button>
            </div>
          )}
          {result && !result.valid && (
            <div className="result-bad">
              <h2>✕ Pase no válido</h2>
              {rejectText(result, when) ? (
                <p data-testid="reject-reason" style={{ marginTop: 6 }}>
                  <b>{rejectText(result, when)}</b>
                </p>
              ) : (
                <>
                  <p>{result.message}</p>
                  <p style={{ marginTop: 8 }}>Motivos posibles:</p>
                  <ul>
                    <li>Vencido</li>
                    <li>Revocado o ya utilizado</li>
                    <li>Inexistente</li>
                    <li>Fuera del horario autorizado</li>
                  </ul>
                </>
              )}
            </div>
          )}
        </div>
      </div>
    </>
  );
}

export function RegisterVisitorPage() {
  const { session, profile } = useAuth();
  const [form, setForm] = useState({ name: '', doc: '', type: 'VISITOR' as VisitorType, unit: '', plate: '', notes: '' });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<ValidateResult | null>(null);
  const set = <K extends keyof typeof form>(k: K, v: (typeof form)[K]) => setForm((f) => ({ ...f, [k]: v }));

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!form.name.trim() || !form.doc.trim()) return setError('Nombre y documento son obligatorios');
    setBusy(true);
    setError(null);
    setResult(null);
    try {
      const r = await accessApi.validatePreAuth(session!.complexId, form.doc);
      setResult(r);
      addLogEntry({
        movement: r.valid ? 'ENTRY' : 'ATTEMPT',
        result: r.valid ? 'AUTHORIZED' : 'DENIED',
        visitor: form.name.trim(),
        unit: form.unit.trim() || unitLabel(r.unit_id, r.unit_name ?? r.access_grant?.unit_name),
        visitorType: VISITOR_TYPE_LABEL[form.type],
        vehicle: form.plate.trim() || undefined,
        registeredBy: profile!.name,
        grantId: r.access_grant?.id,
      });
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <PageHead title="Registrar visitante" subtitle="Completa los datos y valida la autorización" />
      <form className="card form" onSubmit={submit} noValidate>
        <Field label="Nombre">
          <input value={form.name} onChange={(e) => set('name', e.target.value)} placeholder="Juan Pérez" maxLength={200} />
        </Field>
        <Field label="Documento">
          <input value={form.doc} onChange={(e) => set('doc', e.target.value)} placeholder="1.020.345.678" maxLength={50} />
        </Field>
        <Field label="Tipo de visitante">
          <select value={form.type} onChange={(e) => set('type', e.target.value as VisitorType)}>
            {VISITOR_TYPES.map((t) => (
              <option key={t} value={t}>
                {VISITOR_TYPE_LABEL[t]}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Residente / unidad destino">
          <input value={form.unit} onChange={(e) => set('unit', e.target.value)} placeholder="Torre 3, Apto 502" />
        </Field>
        <Field label="Placa (si aplica)">
          <input value={form.plate} onChange={(e) => set('plate', e.target.value)} placeholder="ABC123" maxLength={10} />
        </Field>
        <Field label="Observaciones">
          <input value={form.notes} onChange={(e) => set('notes', e.target.value)} placeholder="Ingresa con vehículo" />
        </Field>
        <div className="field full">
          {error && <ErrorBox message={error} />}
          {result?.valid && (
            <div className="alert green">✓ Autorización activa encontrada · ingreso registrado</div>
          )}
          {result && !result.valid && (
            <div className="alert red">
              <b>Sin autorización vigente</b>
              {rejectText(result, when) ?? result.message} El intento quedó registrado como denegado.
            </div>
          )}
        </div>
        <div className="field full">
          <div>
            <button className="btn" disabled={busy}>
              {busy ? 'Validando…' : 'Registrar visitante'}
            </button>
          </div>
        </div>
      </form>
    </>
  );
}

export function MovementPage() {
  const { profile } = useAuth();
  const loc = useLocation();
  const prefill = (loc.state ?? null) as MovementState | null;
  const [visitor, setVisitor] = useState(prefill?.visitor ?? '');
  const [unit, setUnit] = useState(prefill?.unit ?? '');
  const [type, setType] = useState<VisitorType>(prefill?.visitorType ?? 'VISITOR');
  const [vehicle, setVehicle] = useState(prefill?.vehicle ?? '');
  const [done, setDone] = useState<{ movement: Movement; at: string } | null>(null);
  const [error, setError] = useState<string | null>(null);

  function register(movement: 'ENTRY' | 'EXIT') {
    if (!visitor.trim() || !unit.trim()) return setError('Indica el visitante y la unidad');
    setError(null);
    const e = addLogEntry({
      movement,
      result: 'AUTHORIZED',
      visitor: visitor.trim(),
      unit: unit.trim(),
      visitorType: VISITOR_TYPE_LABEL[type],
      vehicle: vehicle.trim() || undefined,
      registeredBy: profile!.name,
      grantId: prefill?.grantId,
    });
    setDone({ movement, at: e.at });
  }

  return (
    <>
      <PageHead title="Registrar ingreso / salida" subtitle="Confirma los datos antes de registrar" />
      <div className="card form">
        <Field label="Visitante">
          <input value={visitor} onChange={(e) => setVisitor(e.target.value)} placeholder="Juan Pérez" />
        </Field>
        <Field label="Unidad">
          <input value={unit} onChange={(e) => setUnit(e.target.value)} placeholder="Torre 3, Apto 502" />
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
        <Field label="Vehículo / placa">
          <input value={vehicle} onChange={(e) => setVehicle(e.target.value)} placeholder="Automóvil · ABC123" />
        </Field>
        <div className="field full">
          <div className="btn-row" style={{ marginTop: 0 }}>
            <button className="btn" onClick={() => register('ENTRY')}>
              Registrar ingreso
            </button>
            <button className="btn secondary" onClick={() => register('EXIT')}>
              Registrar salida
            </button>
          </div>
        </div>
      </div>
      {error && <div style={{ marginTop: 14 }}><ErrorBox message={error} /></div>}
      {done && (
        <div className="alert green" style={{ marginTop: 14 }}>
          ✓ {done.movement === 'ENTRY' ? 'Ingreso' : 'Salida'} registrado correctamente · {fmtTime(done.at)}
        </div>
      )}
      <div className="btn-row">
        <Link className="btn ghost" to="/">
          ← Volver al inicio
        </Link>
      </div>
    </>
  );
}
