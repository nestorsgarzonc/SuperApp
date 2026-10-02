import { useMemo, useState } from 'react';
import { fmtDay, fmtTime } from '../../../shared/format';
import { Badge, ErrorBox, Loading, PageHead } from '../../../shared/ui';
import { movementLabel, useAccessLog, type LogResult } from '../accessLog';
import { ResultBadge } from '../components/badges';

const uniq = (xs: string[]) => [...new Set(xs)].sort();

export function HistoryPage() {
  const { entries: log, loading, error, reload } = useAccessLog();
  const [day, setDay] = useState<'today' | 'all'>('today');
  const [visitor, setVisitor] = useState('');
  const [unit, setUnit] = useState('');
  const [type, setType] = useState('');
  const [status, setStatus] = useState<'' | LogResult>('');

  const rows = useMemo(() => {
    const today = new Date().toDateString();
    return log.filter(
      (e) =>
        (day === 'all' || new Date(e.at).toDateString() === today) &&
        (!visitor || e.visitor === visitor) &&
        (!unit || e.unit === unit) &&
        (!type || e.visitorType === type) &&
        (!status || e.result === status),
    );
  }, [log, day, visitor, unit, type, status]);

  return (
    <>
      <PageHead title="Historial de accesos" subtitle="Trazabilidad de ingresos y salidas del conjunto" />
      <div className="filters">
        <select aria-label="Fecha" value={day} onChange={(e) => setDay(e.target.value as 'today' | 'all')}>
          <option value="today">Hoy</option>
          <option value="all">Todo el historial</option>
        </select>
        <select aria-label="Visitante" value={visitor} onChange={(e) => setVisitor(e.target.value)}>
          <option value="">Visitante: Todos</option>
          {uniq(log.map((e) => e.visitor)).map((v) => (
            <option key={v}>{v}</option>
          ))}
        </select>
        <select aria-label="Unidad" value={unit} onChange={(e) => setUnit(e.target.value)}>
          <option value="">Unidad: Todas</option>
          {uniq(log.map((e) => e.unit)).map((v) => (
            <option key={v}>{v}</option>
          ))}
        </select>
        <select aria-label="Tipo" value={type} onChange={(e) => setType(e.target.value)}>
          <option value="">Tipo: Todos</option>
          {uniq(log.map((e) => e.visitorType)).map((v) => (
            <option key={v}>{v}</option>
          ))}
        </select>
        <select aria-label="Estado" value={status} onChange={(e) => setStatus(e.target.value as '' | LogResult)}>
          <option value="">Estado: Todos</option>
          <option value="AUTHORIZED">Autorizado</option>
          <option value="DENIED">Denegado</option>
        </select>
      </div>

      {error && <ErrorBox message={error} onRetry={reload} />}
      {loading && log.length === 0 && <Loading />}
      <div className="card table-wrap" style={{ padding: 0 }}>
        {rows.length === 0 ? (
          <div className="empty">No hay movimientos con estos filtros.</div>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Fecha</th>
                <th>Hora</th>
                <th>Visitante</th>
                <th>Unidad</th>
                <th>Tipo</th>
                <th>Movimiento</th>
                <th>Estado</th>
                <th>Registrado por</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((e) => (
                <tr key={e.id}>
                  <td>{fmtDay(e.at)}</td>
                  <td>{fmtTime(e.at)}</td>
                  <td>{e.visitor}</td>
                  <td>{e.unit}</td>
                  <td>{e.visitorType}</td>
                  <td>{movementLabel(e)}</td>
                  <td>
                    <ResultBadge result={e.result} />{' '}
                    {!e.synced && <Badge tone="amber">Sin sincronizar</Badge>}
                  </td>
                  <td>{e.registeredBy}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </>
  );
}
