const tz = 'es-CO';

export const fmtTime = (iso?: string | null) =>
  iso ? new Date(iso).toLocaleTimeString(tz, { hour: 'numeric', minute: '2-digit', hour12: true }) : '—';

const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();

export function fmtDay(iso?: string | null) {
  if (!iso) return '—';
  const d = new Date(iso);
  const diff = Math.round((startOfDay(d) - startOfDay(new Date())) / 86_400_000);
  if (diff === 0) return 'Hoy';
  if (diff === 1) return 'Mañana';
  if (diff === -1) return 'Ayer';
  return d.toLocaleDateString(tz, { day: 'numeric', month: 'short' });
}

export function fmtRange(from?: string | null, until?: string | null) {
  if (!until) return '—';
  const start = from ?? until;
  const sameDay = new Date(start).toDateString() === new Date(until).toDateString();
  // Si termina otro día se dice cuál, p. ej. "Hoy, 2:33 p. m. – Mañana, 2:33 p. m."
  return sameDay
    ? `${fmtDay(start)}, ${fmtTime(from)} – ${fmtTime(until)}`
    : `${fmtDay(start)}, ${fmtTime(from)} – ${fmtDay(until)}, ${fmtTime(until)}`;
}

export const fmtLongDate = (iso: string) =>
  new Date(iso).toLocaleDateString(tz, { day: 'numeric', month: 'long' });

export const toLocalInput = (d: Date) => {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
};
