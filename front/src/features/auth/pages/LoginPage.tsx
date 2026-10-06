import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { LockKeyhole, ShieldCheck } from 'lucide-react';
import { useAuth } from '../components/AuthContext';
import { BrandPanel } from '../components/BrandPanel';
import { loginSchema, type LoginValues } from '../schemas/authSchemas';
import { Button } from '../../../components/ui/Button';
import { Input } from '../../../components/ui/Input';
import { PasswordInput } from '../../../components/ui/PasswordInput';
import { Logo } from '../../../components/ui/Logo';

const copy = { credentials: 'No pudimos validar tus credenciales. Verifica tus datos e inténtalo nuevamente.', network: 'No pudimos conectarnos. Revisa tu conexión e inténtalo nuevamente.', generic: 'No fue posible iniciar sesión. Inténtalo nuevamente.' };
export function LoginPage() {
  const { signIn, loading } = useAuth();
  const navigate = useNavigate();
  const [notice, setNotice] = useState('');
  const form = useForm<LoginValues>({ resolver: zodResolver(loginSchema), defaultValues: { email: '', password: '', remember: false }, mode: 'onSubmit' });
  const submit = form.handleSubmit(async ({ email, password, remember }) => {
    setNotice('');
    try { await signIn(email, password, remember); }
    catch (error) {
      const status = typeof error === 'object' && error !== null && 'status' in error ? error.status : undefined;
      const message = typeof error === 'object' && error !== null && 'message' in error ? error.message : '';
      if (status === 423) { navigate('/account-blocked'); return; }
      setNotice(status === 401 ? copy.credentials : status === 0 ? copy.network : (typeof message === 'string' && message ? message : copy.generic));
    }
  });
  return <main className="auth-layout">
    <BrandPanel />
    <section className="auth-main" aria-labelledby="login-title">
      <header className="auth-mobile-brand"><Logo variant="login" /><span className="brand-panel__tag">GESTIÓN RESIDENCIAL</span></header>
      <div className="auth-card">
        <div className="auth-card__icon"><LockKeyhole size={23} strokeWidth={1.8} /></div>
        <p className="eyebrow">QUÉ BUENO TENERTE DE VUELTA</p>
        <h2 id="login-title">Inicia sesión</h2>
        <p className="auth-card__intro">Ingresa tus datos para continuar a tu comunidad.</p>
        <form onSubmit={submit} noValidate>
          {notice && <div className="alert alert--error" role="alert" aria-live="polite"><span aria-hidden="true">✕</span><span>{notice}{notice === copy.network && <button className="alert__retry" type="button" onClick={() => void submit()}>Intentar nuevamente</button>}</span></div>}
          <Input label="Correo electrónico" placeholder="nombre@correo.com" type="email" autoComplete="email" inputMode="email" autoCapitalize="none" autoCorrect="off" {...form.register('email')} error={form.formState.errors.email?.message} />
          <div className="password-row"><PasswordInput label="Contraseña" placeholder="Ingresa tu contraseña" autoComplete="current-password" {...form.register('password')} error={form.formState.errors.password?.message} /><Link className="text-link" to="/forgot-password">¿Olvidaste tu contraseña?</Link></div>
          <label className="check-row"><input type="checkbox" {...form.register('remember')} /><span className="check-box" aria-hidden="true" /><span>Recordarme</span></label>
          <Button type="submit" className="login-submit" loading={loading}>Iniciar sesión <span aria-hidden="true">→</span></Button>
        </form>
        <div className="secure-note"><ShieldCheck size={17} /><span>Tu información está protegida y segura</span></div>
      </div>
      <footer className="auth-footer"><a href="#tratamiento" onClick={(event) => { event.preventDefault(); setNotice('Consulta nuestro aviso de tratamiento de datos personales conforme a la Ley 1581 de 2012.') }}>Aviso de tratamiento de datos</a><span>·</span><span>© 2026 SuperApp</span></footer>
    </section>
  </main>;
}
