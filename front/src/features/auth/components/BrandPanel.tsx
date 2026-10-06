import { Building2, CalendarDays, MessageCircle, ShieldCheck } from 'lucide-react';
import { Logo } from '../../../components/ui/Logo';
export function BrandPanel() {
  return <aside className="brand-panel" aria-label="SuperApp, gestión residencial">
    <div className="brand-panel__top"><Logo variant="login" /><span className="brand-panel__tag">GESTIÓN RESIDENCIAL</span></div>
    <div className="brand-panel__content">
      <div className="brand-panel__eyebrow"><span className="brand-panel__pulse" /> Tu hogar, mejor conectado</div>
      <h1>La vida en comunidad,<br /><span>más simple.</span></h1>
      <p>Todo lo que pasa en tu conjunto, en un solo lugar.</p>
      <div className="brand-panel__cards" aria-hidden="true">
        <div className="brand-panel__card brand-panel__card--message"><span className="brand-panel__card-icon"><MessageCircle size={19} /></span><span><strong>Tu comunidad te espera</strong><small>Novedades de tu conjunto</small></span><i /></div>
        <div className="brand-panel__card brand-panel__card--reserve"><span className="brand-panel__card-icon"><CalendarDays size={18} /></span><span><strong>Reserva en segundos</strong><small>Espacios comunes, sin llamadas</small></span></div>
        <div className="brand-panel__building"><Building2 size={94} strokeWidth={0.8} /></div>
        <div className="brand-panel__shield"><ShieldCheck size={20} /><span>Tu información,<br />siempre protegida</span></div>
      </div>
    </div>
    <div className="brand-panel__footer"><span>© 2026 SuperApp</span><span>Hecho para vivir mejor</span></div>
  </aside>;
}
