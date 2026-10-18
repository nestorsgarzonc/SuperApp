// Circuit breaker del sistema QR vista desde la interfaz.
// Requiere el ms de PRUEBAS con tiempos cortos:
//   QR_CALL_TIMEOUT_MS=300 QR_BREAKER_OPEN_SECONDS=4 QR_BREAKER_FAILURES=3 ENV=development
// y `npm run dev` apuntando a él. Reiniciar los datos de prueba antes de correr.
import { chromium } from 'playwright-core';
import path from 'node:path';
import { mkdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const OUT = fileURLToPath(new URL('./shots/', import.meta.url));
mkdirSync(OUT, { recursive: true });
const APP = process.env.APP_URL ?? 'http://localhost:5173';
// La sesión de prueba vive en memoria: navegar sin recargar la página para no perderla.
const go = async (page, path) => { await page.evaluate((p) => { history.pushState({}, '', p); dispatchEvent(new PopStateEvent('popstate')); }, path); };
// Estas pruebas son de control-de-accesos: la campana de notificaciones (otro ms) responde vacía.
const stubNotifications = (page) => page.route('**/api/v1/notifications/**', (r) => r.fulfill({ json: { success: true, data: { count: 0 } } }));
let fails = 0, total = 0;
const ok = (n, c, x = '') => { total++; if (!c) fails++; console.log(`${c ? 'PASS' : 'FAIL'}  ${n}${x ? '  → ' + x : ''}`); };
const section = (t) => console.log(`\n== ${t}`);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const browser = await chromium.launch({ channel: process.env.E2E_CHANNEL ?? 'msedge', headless: true });
const errors = [];
const open = async (name, viewport = { width: 390, height: 844 }) => {
  const ctx = await browser.newContext({ serviceWorkers: 'block', viewport });
  const p = await ctx.newPage();
  await stubNotifications(p);
  p.on('pageerror', (e) => errors.push(`${name}: ${e.message}`));
  p.on('console', (m) => m.type() === 'error' && !/favicon/.test(m.location().url) && errors.push(`${name}: ${m.text()}`));
  await p.goto(APP + '/dev-login');
  await p.getByTestId('dev-login').getByRole('button', { name: new RegExp(name) }).click();
  await p.waitForSelector('.shell');
  return p;
};
const panel = (p) => p.getByTestId('dev-panel');
const qrMode = (p) => p.getByTestId('dev-qr-mode');
const qrState = (p) => p.getByTestId('dev-qr-state');
const notice = (p) => p.getByTestId('qr-down-notice');
// El panel está plegado por defecto (no tapa la app): se despliega, se actúa y se pliega.
const devAction = async (p, name) => {
  const head = panel(p).locator('.devpanel-head');
  if ((await head.getAttribute('aria-expanded')) !== 'true') await head.click();
  await p.getByRole('button', { name }).click();
  await head.click();
};

// Partir siempre de "operativo y circuito cerrado" (una corrida anterior pudo dejarlo caído).
await fetch(APP + '/api/v1/dev/qr-system', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ mode: 'UP' }) });
await fetch(APP + '/api/v1/dev/qr-system/reset', { method: 'POST' });

section('Panel de desarrollo');
const ana = await open('Ana García');
await panel(ana).waitFor();
ok('el panel de desarrollo muestra "Sistema QR: operativo" y el circuito cerrado', (await qrMode(ana).innerText()) === 'operativo' && (await qrState(ana).innerText()) === 'cerrado');
ok('sin avisos mientras todo funciona', (await notice(ana).count()) === 0);
await ana.screenshot({ path: path.join(OUT, '30-dev-panel-operativo.png') });

section('Se cae el sistema QR (el botón del panel de desarrollo)');
await devAction(ana, 'Simular caída del sistema QR');
await qrMode(ana).filter({ hasText: 'CAÍDO' }).waitFor();
ok('el panel marca CAÍDO; el circuito sigue cerrado (aún no ha fallado nada)', (await qrState(ana).innerText()) === 'cerrado');

await go(ana, '/autorizaciones/nueva');
await ana.getByPlaceholder('Juan Pérez').fill('Invitado con QR caído');
const qrPill = ana.getByRole('radio', { name: 'Pase con QR' });
ok('con el circuito cerrado el residente aún puede elegir QR', await qrPill.isEnabled());

// 1.er intento: el sistema no responde → se corta por timeout y el front ofrece el fallback
const t0 = Date.now();
await ana.getByRole('button', { name: 'Generar pase' }).click();
await ana.getByText(/El sistema de QR no está disponible/).waitFor();
const took = Date.now() - t0;
ok('el intento NO se queda colgado: responde en ~1 s (timeout)', took < 3000, `${took} ms`);
ok('el front cambia solo a "Por documento" y explica qué hacer', (await ana.getByRole('radio', { name: 'Por documento' }).getAttribute('aria-checked')) === 'true' && (await ana.getByRole('button', { name: 'Autorizar visitante' }).count()) === 1);
await ana.screenshot({ path: path.join(OUT, '31-qr-fallo-fallback.png') });

// 2.º y 3.er intento por QR para abrir el circuito
for (let i = 0; i < 2; i++) {
  await ana.getByRole('radio', { name: 'Pase con QR' }).click();
  await ana.getByRole('button', { name: 'Generar pase' }).click();
  await ana.getByText(/El sistema de QR no está disponible/).waitFor();
}
await qrState(ana).filter({ hasText: 'ABIERTO' }).waitFor({ timeout: 8000 });
ok('tras 3 fallos el circuito se ABRE (se ve en el panel)', (await qrState(ana).innerText()) === 'ABIERTO');
await notice(ana).waitFor({ timeout: 8000 });
ok('el residente ve el aviso: "El sistema de QR no está funcionando… autoriza por documento"', (await notice(ana).innerText()).includes('pre-autorización'));
ok('la opción "Pase con QR" queda deshabilitada y se fuerza "Por documento"', !(await ana.getByRole('radio', { name: 'Pase con QR' }).isEnabled()) && (await ana.getByRole('radio', { name: 'Por documento' }).getAttribute('aria-checked')) === 'true');
await ana.screenshot({ path: path.join(OUT, '32-residente-qr-caido.png') });

// con el circuito abierto, intentar crear algo debe ser instantáneo; y la pre-autorización funciona
await ana.getByPlaceholder('1.020.345.678').fill('COL810001');
const t1 = Date.now();
await ana.getByRole('button', { name: 'Autorizar visitante' }).click();
await ana.getByRole('heading', { name: 'Pase de acceso' }).waitFor();
ok('la pre-autorización por documento SÍ se crea con el QR caído', (await ana.getByText('Documento: COL810001').count()) > 0 && Date.now() - t1 < 3000, `${Date.now() - t1} ms`);

await go(ana, '/');
await notice(ana).waitFor();
ok('el aviso también aparece en el inicio del residente', true);

section('Portería con el sistema QR caído');
const luis = await open('Luis Mora');
await notice(luis).waitFor({ timeout: 8000 });
ok('el guardia ve el aviso en su inicio', (await notice(luis).innerText()).includes('Valida los ingresos por documento'));
await luis.getByRole('link', { name: /VALIDAR PASE/ }).click();
await luis.getByRole('button', { name: 'Validar por documento' }).waitFor();
ok('Validar pase: aviso + botón "Validar por documento"', await notice(luis).isVisible());
ok('el botón "Validar pase" del modo QR está deshabilitado', !(await luis.getByRole('button', { name: 'Validar pase' }).isEnabled()));
await luis.screenshot({ path: path.join(OUT, '33-guardia-qr-caido.png') });
await luis.getByRole('button', { name: 'Validar por documento' }).click();
await luis.getByPlaceholder('1.020.345.678').fill('COL810001');
await luis.getByRole('button', { name: 'Validar pase' }).click();
await luis.getByText('✓ Pase válido').waitFor();
ok('el guardia valida por documento y el ingreso es válido', (await luis.getByText('A-101').count()) > 0);
await luis.screenshot({ path: path.join(OUT, '34-guardia-valida-por-documento.png') });

section('Recuperación');
await devAction(ana, 'Restablecer sistema QR');
await qrMode(ana).filter({ hasText: 'operativo' }).waitFor();
ok('"Restablecer sistema QR" → el panel marca operativo', (await qrMode(ana).innerText()) === 'operativo');
// el circuito se recupera solo tras el enfriamiento (4 s): primero semiabierto, el aviso desaparece
await notice(ana).waitFor({ state: 'detached', timeout: 15000 });
ok('pasado el enfriamiento desaparece el aviso y se puede volver a usar QR', true);
await go(ana, '/autorizaciones/nueva');
await ana.getByPlaceholder('Juan Pérez').fill('Invitado tras la recuperación');
await ana.getByRole('radio', { name: 'Pase con QR' }).click();
ok('la opción "Pase con QR" vuelve a estar habilitada', await ana.getByRole('radio', { name: 'Pase con QR' }).isEnabled());
await ana.getByRole('button', { name: 'Generar pase' }).click();
await ana.getByRole('heading', { name: 'Pase de acceso' }).waitFor();
await ana.locator('.pass-qr img').waitFor();
ok('crea el QR (este intento real cierra el circuito)', (await ana.locator('.pass .code').innerText()).length > 20);
await qrState(ana).filter({ hasText: 'cerrado' }).waitFor({ timeout: 8000 });
ok('el circuito queda CERRADO', (await qrState(ana).innerText()) === 'cerrado');
await go(luis, '/porteria/validar');
await luis.getByRole('tab', { name: /Código QR/ }).click();
const code = (await ana.locator('.pass .code').innerText()).replace('Código:', '').trim();
await luis.getByPlaceholder('SA-7K29-4QX1').fill(code);
await luis.getByRole('button', { name: 'Validar pase' }).click();
await luis.getByText('✓ Pase válido').waitFor();
ok('el guardia vuelve a validar por QR', true);
await ana.screenshot({ path: path.join(OUT, '35-recuperado.png') });

const real = errors.filter((e) => !/status of (403|409|422|401|503)|ERR_FAILED/.test(e));
ok('sin errores de consola/página (salvo los 503 esperados)', real.length === 0, real.slice(0, 3).join(' | '));
await browser.close();
console.log(`\n${total - fails}/${total} OK` + (fails ? `  —  ${fails} FALLAS` : ''));
process.exit(fails ? 1 : 0);
