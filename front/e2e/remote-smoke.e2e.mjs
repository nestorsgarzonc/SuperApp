// Smoke del front publicado (GitHub Pages) contra el API Gateway real. Solo lectura:
// no inicia sesión ni crea datos (el login fallido usa un correo inexistente).
//   E2E_CHANNEL=chrome node e2e/remote-smoke.e2e.mjs
import { chromium } from 'playwright-core';
import path from 'node:path';
import { mkdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const OUT = fileURLToPath(new URL('./shots/', import.meta.url));
mkdirSync(OUT, { recursive: true });
const SITE = (process.env.REMOTE_URL ?? 'https://nestorsgarzonc.github.io/SuperApp').replace(/\/$/, '');
const API = (process.env.REMOTE_API ?? 'https://7fn1lwfutc.execute-api.us-east-2.amazonaws.com/dev/api/v1').replace(/\/$/, '');
let fails = 0, total = 0;
const ok = (n, c, x = '') => { total++; if (!c) fails++; console.log(`${c ? 'PASS' : 'FAIL'}  ${n}${x ? '  → ' + x : ''}`); };
const warn = (n, c, x = '') => { if (!c) console.log(`WARN  ${n}${x ? '  → ' + x : ''}`); };
const section = (t) => console.log(`\n== ${t}`);

const browser = await chromium.launch({ channel: process.env.E2E_CHANNEL ?? 'msedge', headless: true });
const ctx = await browser.newContext({ serviceWorkers: 'block', viewport: { width: 390, height: 844 } });
const page = await ctx.newPage();
const errors = [], badAssets = [], apiCalls = [];
page.on('pageerror', (e) => errors.push(e.message));
page.on('console', (m) => m.type() === 'error' && errors.push(m.text()));
page.on('response', (r) => {
  const u = r.url();
  if (u.startsWith(API)) apiCalls.push({ url: u, status: r.status(), method: r.request().method() });
  else if (u.startsWith(SITE) && r.request().resourceType() !== 'document' && r.status() >= 400) badAssets.push(`${r.status()} ${u}`);
});
const pathname = () => new URL(page.url()).pathname;

section('Sitio publicado');
const res = await page.goto(SITE + '/');
ok('la raíz responde 200', res?.status() === 200, String(res?.status()));
await page.waitForURL(/\/SuperApp\/login$/, { timeout: 10000 }).catch(() => {});
ok('/ → splash → /SuperApp/login', pathname() === '/SuperApp/login', pathname());
await page.getByRole('button', { name: /Iniciar sesión/ }).waitFor();
ok('la pantalla de login se dibuja', await page.getByLabel('Correo electrónico').isVisible());
ok('JS/CSS/imágenes cargan bajo /SuperApp (sin 404)', badAssets.length === 0, badAssets.slice(0, 3).join(' | '));
await page.screenshot({ path: path.join(OUT, '40-remoto-login.png') });

section('Rutas profundas (404.html de Pages)');
await page.goto(SITE + '/forgot-password');
await page.getByRole('heading', { name: '¿Olvidaste tu contraseña?' }).waitFor({ timeout: 10000 }).catch(() => {});
ok('entrar directo a /forgot-password carga la SPA', await page.getByRole('heading', { name: '¿Olvidaste tu contraseña?' }).isVisible());
await page.goto(SITE + '/historial');
await page.waitForURL(/\/SuperApp\/login$/, { timeout: 10000 }).catch(() => {});
ok('ruta protegida sin sesión → login', pathname() === '/SuperApp/login', pathname());
await page.goto(SITE + '/dev-login');
await page.waitForTimeout(1500);
// El equipo lo habilitó a propósito para la demo (VITE_ENABLE_DEV_LOGIN): se avisa, no falla.
warn('el login de desarrollo NO existe en producción', (await page.getByTestId('dev-login').count()) === 0);

section('Login (sin credenciales reales)');
await page.goto(SITE + '/login');
await page.getByRole('button', { name: /Iniciar sesión/ }).waitFor();
const before = apiCalls.length;
await page.getByRole('button', { name: /Iniciar sesión/ }).click();
await page.waitForTimeout(500);
ok('formulario vacío: valida sin llamar al servidor', apiCalls.length === before && (await page.locator('.field-error, [role=alert], [aria-invalid=true]').count()) > 0);
await page.getByLabel('Correo electrónico').fill('e2e-smoke@example.invalid');
await page.getByLabel('Contraseña', { exact: true }).fill('no-es-una-clave');
const loginResp = page.waitForResponse((r) => r.url().startsWith(API + '/auth/login'), { timeout: 35000 }).catch(() => null);
await page.getByRole('button', { name: /Iniciar sesión/ }).click();
const lr = await loginResp;
ok('el login va al API Gateway (no a github.io)', !!lr, lr ? lr.url() : 'sin respuesta del gateway');
ok('el gateway responde 401 al login inválido (CORS OK)', lr?.status() === 401, String(lr?.status()));
await page.getByRole('alert').waitFor({ timeout: 10000 }).catch(() => {});
ok('se muestra "No pudimos validar tus credenciales"', (await page.getByText(/No pudimos validar tus credenciales/).count()) === 1, (await page.getByRole('alert').allInnerTexts()).join(' | '));
await page.screenshot({ path: path.join(OUT, '41-remoto-login-invalido.png') });

section('Microservicios vía gateway, desde el navegador (CORS real)');
// fetch desde la página: si CORS falla, el navegador no deja leer la respuesta (status 0).
const call = (p, init = {}) => page.evaluate(async ([u, i]) => {
  const t = performance.now();
  try { const r = await fetch(u, i); return { status: r.status, ms: Math.round(performance.now() - t) }; }
  catch (e) { return { status: 0, ms: Math.round(performance.now() - t), err: String(e) }; }
}, [API + p, init]);
const authed = { headers: { Authorization: 'Bearer token-invalido', 'X-Complex-ID': '00000000-0000-0000-0000-000000000001' } };
const checks = [
  ['auth: /auth/refresh sin cookie → 400/401', '/auth/refresh', { method: 'POST', headers: { 'Content-Type': 'application/json' } }, [400, 401]],
  ['control-de-accesos: /access-logs con token inválido → 401', '/access-logs', authed, [401]],
  ['notificaciones: /notifications/unread-count con token inválido → 401', '/notifications/unread-count', authed, [401]],
];
for (const [name, p, init, want] of checks) {
  const r = await call(p, init);
  ok(name, want.includes(r.status), r.status === 0 ? `bloqueado/red (${r.ms} ms) ${r.err ?? ''}` : `${r.status} en ${r.ms} ms`);
}

section('Seguridad');
const dev = await call('/dev/users');
warn('endpoints /dev/* NO expuestos en el gateway', dev.status !== 200, `/dev/users → ${dev.status}`);

const real = errors.filter((e) => !/status of 40[01]|status of 504|Failed to load resource/.test(e));
ok('sin errores de consola/página (salvo 400/401 esperados)', real.length === 0, real.slice(0, 3).join(' | '));
await browser.close();
console.log(`\n${total - fails}/${total} OK` + (fails ? `  —  ${fails} FALLAS` : ''));
process.exit(fails ? 1 : 0);
