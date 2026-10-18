// E2E completo contra el front PUBLICADO (GitHub Pages) + API Gateway real, con login real
// de las personas demo (auth: db/dev-seeds/001_demo.sql, mismos UUID que control-de-accesos).
// Dos modos de entrada:
//   - login real (auth): E2E_PASSWORD='…' E2E_CHANNEL=chrome node e2e/remote.e2e.mjs
//   - sin E2E_PASSWORD: /dev-login del sitio publicado (VITE_ENABLE_DEV_LOGIN=true en la demo).
// Crea datos en el entorno remoto (pases, movimientos, registros); cada corrida usa nombres únicos.
import { chromium } from 'playwright-core';
import path from 'node:path';
import { mkdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const PASSWORD = process.env.E2E_PASSWORD;
const MODE = PASSWORD ? 'login real (auth)' : '/dev-login';
const OUT = fileURLToPath(new URL('./shots/', import.meta.url));
mkdirSync(OUT, { recursive: true });
const SITE = (process.env.REMOTE_URL ?? 'https://nestorsgarzonc.github.io/SuperApp').replace(/\/$/, '');
const API = (process.env.REMOTE_API ?? 'https://7fn1lwfutc.execute-api.us-east-2.amazonaws.com/dev/api/v1').replace(/\/$/, '');
const BASE = new URL(SITE).pathname; // '/SuperApp'
const RUN = Date.now().toString(36).slice(-5).toUpperCase();
// Nombres cortos: sirven tanto para "Los Pinos" (directorio demo del front) como para "Conjunto Residencial Los Pinos".
const C1 = 'Los Pinos', C2 = 'Alto del Bosque';
// Elige en el selector del encabezado la opción cuyo texto contiene el nombre del conjunto.
const pickComplex = async (page, name) => {
  const sel = page.locator('select.chip-select');
  const label = (await sel.locator('option').allInnerTexts()).find((t) => t.includes(name));
  await sel.selectOption({ label });
};
const activeComplex = (page) => page.locator('select.chip-select option:checked').innerText();
const NAME = { ana: 'Ana García', pedro: 'Pedro Gómez', luis: 'Luis Mora', marta: 'Marta Ruiz', carlos: 'Carlos Rojas', sofia: 'Sofía', andres: 'Andrés' };
const EMAIL = {
  ana: 'ana.garcia@demo.test', pedro: 'pedro.gomez@demo.test', luis: 'luis.mora@demo.test', marta: 'marta.ruiz@demo.test',
  carlos: 'carlos.rojas@demo.test', sofia: 'sofia.diaz@demo.test', andres: 'andres.vega@demo.test',
};

let fails = 0, total = 0;
const ok = (n, c, x = '') => { total++; if (!c) fails++; console.log(`${c ? 'PASS' : 'FAIL'}  ${n}${x ? '  → ' + x : ''}`); return c; };
const warn = (n, c, x = '') => { if (!c) console.log(`WARN  ${n}${x ? '  → ' + x : ''}`); };
const section = (t) => console.log(`\n== ${t}`);
// Un paso que falla no tumba la corrida: se reporta y se sigue con la siguiente sección.
const step = async (name, fn) => { try { await fn(); } catch (e) { ok(name, false, e.message.split('\n')[0]); } };

const browser = await chromium.launch({ channel: process.env.E2E_CHANNEL ?? 'msedge', headless: true });
const errors = [];
const creds = new Map(); // page → { token, complexId } tomados de las llamadas reales de la app
// Navegar sin recargar: la sesión vive en memoria.
const go = (page, p) => page.evaluate((u) => { history.pushState({}, '', u); dispatchEvent(new PopStateEvent('popstate')); }, BASE + p);
const here = (page) => new URL(page.url()).pathname.replace(BASE, '') || '/';

async function login(who, { complex, viewport = { width: 390, height: 844 }, ctx } = {}) {
  ctx ??= await browser.newContext({ serviceWorkers: 'block', viewport });
  const page = await ctx.newPage();
  page.on('pageerror', (e) => errors.push(`${who}: ${e.message}`));
  // GitHub Pages sirve las rutas profundas con 404.html (status 404) y no tiene /favicon.ico: no son errores de la app.
  page.on('console', (m) => m.type() === 'error' && !(m.location().url.startsWith(new URL(SITE).origin) && /status of 404/.test(m.text())) && errors.push(`${who}: ${m.text()}`));
  page.on('request', (r) => {
    const h = r.headers();
    if (r.url().startsWith(API) && h.authorization?.startsWith('Bearer ') && h['x-complex-id']) creds.set(page, { token: h.authorization.slice(7), complexId: h['x-complex-id'] });
  });
  await enter(page, who, complex);
  return page;
}
async function enter(page, who, complex) {
  if (!PASSWORD) return enterDev(page, who, complex);
  await page.goto(SITE + '/login');
  await page.getByLabel('Correo electrónico').fill(EMAIL[who]);
  await page.getByLabel('Contraseña', { exact: true }).fill(PASSWORD);
  await page.getByRole('button', { name: /Iniciar sesión/ }).click();
  await page.waitForURL((u) => /\/(select-complex|unauthorized)$/.test(u.pathname) || !/\/login$/.test(u.pathname), { timeout: 20000 });
  const alert = page.getByRole('alert');
  if (await alert.count()) throw new Error(`login de ${who}: ${(await alert.allInnerTexts()).join(' ')}`);
  if (here(page) === '/select-complex') {
    if (!complex) throw new Error(`${who} tiene varios conjuntos: falta indicar cuál`);
    await page.locator('.complex-card', { hasText: complex }).click();
    await page.getByRole('button', { name: /Continuar/ }).click();
  }
  await page.waitForSelector('.shell', { timeout: 20000 });
}
async function enterDev(page, who, complex) {
  await page.goto(SITE + '/dev-login');
  await page.getByTestId('dev-login').getByRole('button', { name: new RegExp(NAME[who]) }).click();
  const pick = page.getByRole('heading', { name: 'Selecciona tu conjunto' });
  await Promise.race([pick.waitFor({ timeout: 20000 }), page.waitForSelector('.shell', { timeout: 20000 })]);
  if (await pick.count()) {
    if (!complex) throw new Error(`${who} tiene varios conjuntos: falta indicar cuál`);
    await page.locator('.profile-card', { hasText: complex }).first().click();
  }
  await page.waitForSelector('.shell', { timeout: 20000 });
}
// Llamada directa al ms con la sesión que la app está usando (verificación del lado servidor).
async function api(page, p) {
  const c = creds.get(page);
  if (!c) throw new Error('sin token capturado para esta página');
  const r = await fetch(API + p, { headers: { Authorization: 'Bearer ' + c.token, 'X-Complex-ID': c.complexId } });
  return { status: r.status, data: (await r.json().catch(() => null))?.data };
}
const shot = (page, name) => page.screenshot({ path: path.join(OUT, name + '.png') });
const pending = (page) => page.evaluate(() => JSON.parse(localStorage.getItem('superapp.accessLog.pending') || '[]').length);
// Contra el entorno remoto la sincronización tarda más: se espera a que la cola se vacíe (máx. 20 s).
async function drainQueue(page) {
  for (let i = 0; i < 20 && (await pending(page)) > 0; i++) {
    await page.getByRole('button', { name: 'Sincronizar ahora' }).click({ timeout: 500 }).catch(() => {});
    await page.waitForTimeout(1000);
  }
  return pending(page);
}

// ─────────────────────────────────────────────────────────────────────────────
section(`Entrada de las personas demo (${MODE})`);
let ana, luis, carlos, marta;
await step('Ana inicia sesión', async () => {
  ana = await login('ana');
  ok('Ana (residente, 1 conjunto) entra directo a su inicio', await ana.getByText('Hola, Ana').isVisible(), here(ana));
  ok('el encabezado muestra su conjunto', (await ana.locator('.shell').innerText()).includes(C1));
  await shot(ana, '50-remoto-residente-inicio');
});
await step('Luis inicia sesión', async () => {
  luis = await login('luis');
  await luis.getByText('Operación en tiempo real').waitFor();
  ok('Luis (guardia c1) ve portería y "● Conectado"', await luis.getByText('● Conectado').isVisible());
});
if (PASSWORD) await step('login inválido', async () => {
  const p = await (await browser.newContext({ serviceWorkers: 'block' })).newPage();
  await p.goto(SITE + '/login');
  await p.getByLabel('Correo electrónico').fill(EMAIL.ana);
  await p.getByLabel('Contraseña', { exact: true }).fill('clave-incorrecta-e2e');
  await p.getByRole('button', { name: /Iniciar sesión/ }).click();
  await p.getByText(/No pudimos validar tus credenciales/).waitFor();
  ok('contraseña incorrecta → mensaje de credenciales y sigue en /login', here(p) === '/login');
  await p.context().close();
});
await step('token', async () => {
  const r = await api(ana, '/access-logs?limit=1');
  ok('control-de-accesos acepta el token del ms de auth', r.status !== 401, String(r.status));
});

// ─────────────────────────────────────────────────────────────────────────────
section('Residente autoriza visitante → portería valida → ingreso y salida');
const visitor = `E2E Visita ${RUN}`;
let code;
await step('crear pase', async () => {
  await ana.getByRole('link', { name: /Autorizar visitante/ }).click();
  await ana.getByRole('heading', { name: 'Nueva autorización' }).waitFor();
  await ana.getByRole('button', { name: 'Generar pase' }).click();
  ok('el formulario valida el nombre vacío', await ana.getByText('Escribe el nombre del visitante').isVisible());
  await ana.getByPlaceholder('Juan Pérez').fill(visitor);
  await ana.locator('select').selectOption('DELIVERY');
  const times = ana.locator('input[type=time]');
  await times.nth(0).fill('00:00');
  await times.nth(1).fill('23:59');
  await ana.getByPlaceholder('Automóvil').fill('Moto');
  await ana.getByPlaceholder('ABC123').fill('e2e' + RUN.slice(0, 3));
  await ana.getByRole('button', { name: 'Generar pase' }).click();
  await ana.getByRole('heading', { name: 'Pase de acceso' }).waitFor();
  ok('pase: QR dibujado', (await ana.locator('.pass-qr img').getAttribute('src'))?.startsWith('data:image/png'));
  code = (await ana.locator('.pass .code').innerText()).replace('Código:', '').trim();
  ok('pase: código del backend (64 hex)', /^[0-9a-f]{64}$/.test(code), code.slice(0, 12) + '…');
  await shot(ana, '51-remoto-pase');
  await go(ana, '/autorizaciones');
  await ana.getByText(visitor).first().waitFor();
  ok('el pase aparece en "Autorizaciones" como Activa', (await ana.getByText('✓ Activa').count()) >= 1);
});
await step('portería valida', async () => {
  await luis.getByRole('link', { name: /VALIDAR PASE/ }).click();
  await luis.getByPlaceholder('SA-7K29-4QX1').fill('codigo-que-no-existe');
  await luis.getByRole('button', { name: 'Validar pase' }).click();
  await luis.getByText('✕ Pase no válido').waitFor();
  ok('código inexistente → rechazado', true);
  await luis.getByPlaceholder('SA-7K29-4QX1').fill(code);
  await luis.getByRole('button', { name: 'Validar pase' }).click();
  await luis.getByText('✓ Pase válido').waitFor();
  ok('el código real → válido, con el nombre del visitante', (await luis.getByText(visitor).count()) > 0);
  await luis.getByRole('button', { name: 'Registrar ingreso' }).click();
  await luis.getByRole('heading', { name: /Registrar ingreso/ }).waitFor();
  ok('ingreso prellenado desde el pase', (await luis.locator('input').first().inputValue()) === visitor);
  await luis.getByRole('button', { name: 'Registrar ingreso' }).click();
  await luis.getByText(/Ingreso registrado correctamente/).waitFor();
  await luis.getByRole('button', { name: 'Registrar salida' }).click();
  await luis.getByText(/Salida registrado correctamente/).waitFor();
  await shot(luis, '52-remoto-ingreso-salida');
  ok('cola local vacía tras sincronizar', (await drainQueue(luis)) === 0);
  const logs = (await api(luis, '/access-logs?limit=100')).data ?? [];
  const mine = logs.filter((l) => l.visitor_name === visitor);
  ok('servidor: ENTRY y EXIT del visitante', mine.some((l) => l.movement === 'ENTRY') && mine.some((l) => l.movement === 'EXIT'), `${mine.length} registros`);
});
await step('pase usado', async () => {
  await enter(ana, 'ana'); // recarga real: lista fresca del servidor
  await go(ana, '/autorizaciones');
  await ana.getByRole('heading', { name: 'Autorizaciones' }).waitFor();
  await ana.waitForTimeout(1000);
  ok('el pase usado salió de "Activas"', (await ana.getByText(visitor).count()) === 0);
});

// ─────────────────────────────────────────────────────────────────────────────
section('Portería sin conexión: cola local y sincronización');
const offline = `E2E Offline ${RUN}`;
await step('offline', async () => {
  await go(luis, '/porteria/movimiento');
  await luis.getByPlaceholder('Juan Pérez').waitFor();
  const before = await drainQueue(luis);
  await luis.context().setOffline(true);
  await luis.getByPlaceholder('Juan Pérez').fill(offline);
  await luis.getByPlaceholder('Torre 3, Apto 502').fill('A-101');
  await luis.getByRole('button', { name: 'Registrar ingreso' }).click();
  await luis.getByText(/Ingreso registrado correctamente/).waitFor();
  ok('sin red: el registro queda en cola local', (await pending(luis)) === before + 1, `pendientes=${await pending(luis)}`);
  await luis.context().setOffline(false);
  await go(luis, '/');
  await luis.getByText('Operación en tiempo real').waitFor();
  ok('con red: la cola se sincroniza', (await drainQueue(luis)) === 0);
  const logs = (await api(luis, '/access-logs?limit=100')).data ?? [];
  ok('el servidor lo recibió una sola vez', logs.filter((l) => l.visitor_name === offline).length === 1);
});

// ─────────────────────────────────────────────────────────────────────────────
section('Residente entra y sale del edificio; portería lo ve');
const status = (p) => p.getByTestId('building-status');
const inBtn = (p) => p.getByRole('button', { name: 'Ingresar al edificio' });
const outBtn = (p) => p.getByRole('button', { name: 'Salir del edificio' });
await step('edificio', async () => {
  await go(ana, '/edificio');
  await ana.getByRole('heading', { name: 'Mi acceso al edificio' }).waitFor();
  await ana.waitForTimeout(1000);
  // El entorno remoto no se reinicia: si quedó dentro de una corrida anterior, primero sale.
  if (await status(ana).getByText('Estás dentro del edificio').count()) { await outBtn(ana).click(); await ana.getByText(/Salida registrado/).waitFor(); }
  await inBtn(ana).click();
  await ana.getByText(/Ingreso registrado/).waitFor();
  await status(ana).getByText('Estás dentro del edificio').waitFor({ timeout: 8000 }).catch(() => {});
  ok('ingresa: "Estás dentro del edificio"', await status(ana).getByText('Estás dentro del edificio').isVisible());
  ok('"Ingresar" deshabilitado, "Salir" disponible', !(await inBtn(ana).isEnabled()) && (await outBtn(ana).isEnabled()));
  await go(luis, '/');
  await luis.getByRole('button', { name: 'Actualizar' }).click();
  await luis.getByTestId('presence-card').getByText('Ana García').waitFor({ timeout: 10000 });
  ok('Luis (c1) la ve dentro del edificio con su unidad', (await luis.getByTestId('presence-card').innerText()).includes('A-101'));
  await shot(luis, '53-remoto-presencia');
});
await step('marta', async () => {
  marta = await login('marta');
  await marta.getByTestId('presence-card').waitFor();
  await marta.waitForTimeout(1000);
  ok('Marta (guardia c2) NO ve a Ana', (await marta.getByTestId('presence-card').getByText('Ana García').count()) === 0);
});
await step('salida', async () => {
  await outBtn(ana).click();
  await ana.getByText(/Salida registrado/).waitFor();
  await luis.getByRole('button', { name: 'Actualizar' }).click();
  await luis.waitForTimeout(1500);
  ok('Ana sale → Luis deja de verla', (await luis.getByTestId('presence-card').getByText('Ana García').count()) === 0);
});

// ─────────────────────────────────────────────────────────────────────────────
section('Administración (escritorio)');
await step('admin', async () => {
  carlos = await login('carlos', { viewport: { width: 1280, height: 800 } });
  await carlos.getByText('Panel de administración').waitFor();
  ok('panel con "Accesos hoy"', await carlos.getByText('Accesos hoy').isVisible());
  ok('escritorio: barra lateral visible, barra inferior oculta', (await carlos.locator('.sidebar').isVisible()) && !(await carlos.locator('.bottom-nav').isVisible()));
  await carlos.getByRole('link', { name: 'Ver historial completo' }).click();
  await carlos.getByRole('heading', { name: 'Historial de accesos' }).waitFor();
  await carlos.locator('select[aria-label=Fecha]').selectOption('all').catch(() => {});
  await carlos.locator('table tbody tr').first().waitFor();
  const t = await carlos.locator('table tbody').innerText();
  ok('el historial incluye el visitante de esta corrida', t.includes(visitor));
  ok('y los movimientos de residentes', t.includes('Ingreso al edificio') && t.includes('Ana García'));
  await carlos.locator('select[aria-label=Estado]').selectOption('DENIED');
  ok('filtro Estado = Denegado', (await carlos.locator('table tbody tr').count()) >= 1 && (await carlos.locator('table tbody tr .badge.green').count()) === 0);
  await shot(carlos, '54-remoto-historial');
});

// ─────────────────────────────────────────────────────────────────────────────
section('Permisos por rol');
await step('rbac', async () => {
  for (const p of ['/historial', '/porteria/validar']) {
    await go(ana, p);
    await ana.waitForTimeout(500);
    ok(`residente no entra a ${p}`, here(ana) === '/unauthorized', here(ana));
  }
  await go(luis, '/edificio');
  await luis.waitForTimeout(500);
  ok('guardia no entra a /edificio', here(luis) === '/unauthorized', here(luis));
});

// ─────────────────────────────────────────────────────────────────────────────
section('Varios conjuntos: Sofía (residente en ambos) y Andrés (residente c1, guardia c2)');
await step('sofia', async () => {
  const sofia = await login('sofia', { complex: C1 });
  ok('Sofía elige conjunto al entrar y queda en Los Pinos', (await sofia.locator('.shell').innerText()).includes(C1));
  await pickComplex(sofia, C2);
  await sofia.waitForFunction((n) => document.querySelector('select.chip-select option:checked')?.textContent?.includes(n), C2, { timeout: 15000 });
  await go(sofia, '/');
  await sofia.waitForTimeout(1500);
  ok('cambia a Alto del Bosque desde el encabezado', (await activeComplex(sofia)).includes(C2));
  ok('las llamadas siguientes van con el conjunto 2', creds.get(sofia)?.complexId?.endsWith('0002'), creds.get(sofia)?.complexId);
});
await step('andres', async () => {
  const andres = await login('andres', { complex: C2 });
  await andres.getByText('Operación en tiempo real').waitFor({ timeout: 15000 });
  ok('en Alto del Bosque Andrés entra como PORTERÍA', (await andres.getByRole('link', { name: /VALIDAR PASE/ }).count()) === 1);
  await pickComplex(andres, C1);
  await andres.getByText('Hola, Andrés').waitFor({ timeout: 15000 });
  ok('en Los Pinos entra como RESIDENTE (sin portería)', (await andres.getByRole('link', { name: /VALIDAR PASE/ }).count()) === 0);
  await go(andres, '/porteria/validar');
  await andres.waitForTimeout(500);
  ok('como residente no entra a /porteria/validar', here(andres) === '/unauthorized', here(andres));
});

// ─────────────────────────────────────────────────────────────────────────────
section('Notificaciones');
await step('notificaciones', async () => {
  const r = await api(ana, '/notifications/users/me/notifications/unread-count');
  ok('el ms de notificaciones responde con la sesión de Ana', r.status === 200, String(r.status));
  await go(ana, '/');
  await ana.waitForSelector('.shell');
  ok('la campana se dibuja en el encabezado', (await ana.getByTestId('notification-bell').count()) > 0);
});

// ─────────────────────────────────────────────────────────────────────────────
section('Seguridad');
const dev = await fetch(API + '/dev/users').then((r) => r.status).catch(() => 0);
// Expuestos a propósito para la demo (dev-login en el build publicado): se avisa, no falla.
warn('endpoints /dev/* NO expuestos en el gateway', dev !== 200, `/dev/users → ${dev}`);

const real = errors.filter((e) => !/status of (400|401|403|409|422)|ERR_INTERNET_DISCONNECTED|ERR_FAILED|favicon/.test(e));
ok('sin errores de consola/página (salvo los esperados)', real.length === 0, [...new Set(real)].slice(0, 4).join(' | '));
await browser.close();
console.log(`\nCorrida ${RUN}: ${total - fails}/${total} OK` + (fails ? `  —  ${fails} FALLAS` : ''));
process.exit(fails ? 1 : 0);
