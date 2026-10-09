import { useEffect, useState } from 'react';
import { useAsync } from '../../../shared/useAsync';
import { ErrorBox, Loading, PageHead } from '../../../shared/ui';
import { notificationsApi, type NotificationGroup, type PreferenceItem } from '../api';

const COPY: Record<NotificationGroup, { title: string; detail: string }> = {
  ACCESS_CONTROL: { title: 'Control de acceso', detail: 'Autorizaciones, ingresos y salidas de tus visitantes' },
  INTERCOM: { title: 'Citofonía', detail: 'Llamadas entrantes de portería' },
  RESERVATIONS: { title: 'Reservas', detail: 'Confirmaciones y recordatorios' },
  COMMUNITY: { title: 'Comunicados', detail: 'Anuncios de la administración' },
  ASSEMBLIES: { title: 'Asambleas', detail: 'Convocatorias y resúmenes' },
  VOTES: { title: 'Votaciones', detail: 'Nuevas votaciones y cierres' },
  SYSTEM: { title: 'Sistema', detail: 'Avisos de tu cuenta' },
};

export function NotificationPreferencesPage() {
  const { data, error, loading, reload } = useAsync(() => notificationsApi.preferences(), []);
  const [prefs, setPrefs] = useState<PreferenceItem[] | null>(null);
  const [saving, setSaving] = useState<NotificationGroup | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);

  useEffect(() => {
    if (data) setPrefs(data);
  }, [data]);

  async function change(group: NotificationGroup, enabled: boolean) {
    const previous = prefs;
    setPrefs((p) => p?.map((x) => (x.group === group ? { ...x, enabled } : x)) ?? p);
    setSaving(group);
    setSaveError(null);
    try {
      setPrefs(await notificationsApi.setPreference(group, enabled));
    } catch (e) {
      setPrefs(previous);
      setSaveError((e as Error).message);
    } finally {
      setSaving(null);
    }
  }

  return (
    <>
      <PageHead title="Preferencias de notificaciones" subtitle="Elige qué avisos quieres recibir" />
      {loading && !prefs && <Loading />}
      {error && <ErrorBox message={error} onRetry={reload} />}
      {saveError && (
        <div style={{ marginBottom: 14 }}>
          <ErrorBox message={saveError} />
        </div>
      )}
      {prefs && (
        <div className="card" style={{ maxWidth: 640 }}>
          <ul className="list">
            {prefs.map((p) => {
              const copy = COPY[p.group];
              return (
                <li key={p.group}>
                  <div>
                    <div className="t">{copy.title}</div>
                    <div className="s">
                      {copy.detail}
                      {p.locked && ' · Obligatoria por seguridad'}
                    </div>
                  </div>
                  <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                    <button
                      type="button"
                      role="switch"
                      aria-checked={p.enabled}
                      aria-label={copy.title}
                      className={`switch${p.enabled ? ' on' : ''}${p.locked ? ' locked' : ''}`}
                      disabled={p.locked || saving === p.group}
                      onClick={() => void change(p.group, !p.enabled)}
                    >
                      <span />
                    </button>
                    <span aria-hidden style={{ width: 16, fontSize: 12 }}>
                      {p.locked ? '🔒' : ''}
                    </span>
                  </span>
                </li>
              );
            })}
          </ul>
        </div>
      )}
      {prefs && (
        <p className="s" style={{ color: 'var(--muted)', fontSize: 13, marginTop: 12 }}>
          Los avisos de seguridad no se pueden desactivar.
        </p>
      )}
    </>
  );
}
