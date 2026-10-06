import { useState } from 'react';
export function Logo({ variant = 'header' }: { variant?: 'login' | 'sidebar' | 'header' }) {
  const [broken, setBroken] = useState(false);
  const logoUrl = import.meta.env.VITE_LOGO_URL ?? '/src/assets/brand/logo.svg';
  return <span className={`logo logo--${variant}`}>
    {!broken && <img src={logoUrl} alt="" onError={() => setBroken(true)} />}
    {broken && <span className="logo__symbol" aria-hidden="true">S</span>}
    <span>SuperApp</span>
  </span>;
}
