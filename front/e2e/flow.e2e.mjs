// Requiere: ms de control-de-accesos (ENV=development, build -tags dev, db/seeds/001_demo.sql) y `npm run dev` arriba; Edge/Chrome instalado.
// Reiniciar los datos de prueba antes de correr (consume pases de un solo uso).
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
let fails = 0;
const ok = (n, c, x = '') => { if (!c) fails++; console.log(`${c ? 'PASS' : 'FAIL'}  ${n}${x ? '  → ' + x : ''}`); };

const browser = await chromium.launch({ channel: process.env.E2E_CHANNEL ?? 'msedge', headless: true });
const errors = [];
const watch = (page, tag) => {
  page.on('pageerror', (e) => errors.push(`${tag} pageerror: ${e.message}`));
  page.on('console', (m) => m.type() === 'error' && !/favicon|fonts\.g|ERR_INTERNET_DISCONNECTED|ERR_FAILED/.test(m.text() + ' ' + m.location().url) && errors.push(`${tag} console: ${m.text()}`));
};
const shot = (page, name) => page.screenshot({ path: path.join(OUT, name + '.png'), fullPage: false });

async function login(ctx, who) {
  const page = await ctx.newPage();
  await stubNotifications(page);
  watch(page, who);
  await page.goto(APP + '/dev-login');
  await page.getByRole('button', { name: new RegExp(who) }).click();
  await page.waitForSelector('.shell');
  return page;
}

// ── MÓVIL: residente
const mobile = await browser.newContext({ serviceWorkers: 'block', viewport: { width: 390, height: 844 }, deviceScaleFactor: 2 });
const res = await login(mobile, 'Ana García');
await res.getByText('Autorizaciones activas').waitFor();
ok('residente: inicio', await res.getByText('Hola, Ana').isVisible());
await shot(res, '01-residente-inicio');

await res.getByRole('link', { name: /Autorizar visitante/ }).click();
await res.getByRole('heading', { name: 'Nueva autorización' }).waitFor();
// validación: sin nombre
await res.getByRole('button', { name: 'Generar pase' }).click();
ok('form: valida nombre vacío', await res.getByText('Escribe el nombre del visitante').isVisible());
await res.getByPlaceholder('Juan Pérez').fill('María López');
await res.locator('select').selectOption('DELIVERY');
const times = res.locator('input[type=time]');
await times.nth(0).fill('00:00');
await times.nth(1).fill('23:59');
await res.getByPlaceholder('Automóvil').fill('Moto');
await res.getByPlaceholder('ABC123').fill('xyz98');
await shot(res, '02-nueva-autorizacion');
await res.getByRole('button', { name: 'Generar pase' }).click();
await res.getByRole('heading', { name: 'Pase de acceso' }).waitFor();
const img = res.locator('.pass-qr img');
await img.waitFor();
ok('pase: QR dibujado', (await img.getAttribute('src'))?.startsWith('data:image/png'));
const codeText = await res.locator('.pass .code').innerText();
const code = codeText.replace('Código:', '').trim();
ok('pase: código del backend (64 hex)', /^[0-9a-f]{64}$/.test(code), code.slice(0, 12) + '…');
ok('pase: nombre y vehículo', (await res.getByText('María López').count()) > 0 && (await res.getByText(/Placa: XYZ98/).count()) > 0);
await shot(res, '03-pase-acceso');

await go(res, '/autorizaciones');
await res.getByText('María López').first().waitFor();
ok('lista: aparece el pase nuevo como Activa', (await res.getByText('✓ Activa').count()) >= 1);
await shot(res, '04-autorizaciones');

// ── MÓVIL: portería
const gctx = await browser.newContext({ serviceWorkers: 'block', viewport: { width: 390, height: 844 }, deviceScaleFactor: 2 });
const guard = await login(gctx, 'Luis Mora');
await guard.getByText('Operación en tiempo real').waitFor();
ok('portería: conectado', await guard.getByText('● Conectado').isVisible());
await shot(guard, '05-porteria-inicio');

await guard.getByRole('link', { name: /VALIDAR PASE/ }).click();
await guard.getByPlaceholder('SA-7K29-4QX1').fill('codigo-que-no-existe');
await guard.getByRole('button', { name: 'Validar pase' }).click();
await guard.getByText('✕ Pase no válido').waitFor();
ok('validar: código inexistente → rechazado', true);
// La pantalla de validación no llama al residente (la citofonía es otro módulo, con su propio ítem en la navegación).
ok('validar: SIN citofonía', (await guard.locator('main.content').getByText(/itofon|Llamar residente/i).count()) === 0);
await shot(guard, '06-validar-rechazado');

await guard.getByPlaceholder('SA-7K29-4QX1').fill(code);
await guard.getByRole('button', { name: 'Validar pase' }).click();
await guard.getByText('✓ Pase válido').waitFor();
ok('validar: QR real → válido', (await guard.getByText('María López').count()) > 0);
await shot(guard, '07-validar-valido');
await guard.getByRole('button', { name: 'Registrar ingreso' }).click();
await guard.getByRole('heading', { name: /Registrar ingreso/ }).waitFor();
ok('ingreso: prellenado desde el pase', (await guard.locator('input').first().inputValue()) === 'María López');
await guard.getByRole('button', { name: 'Registrar ingreso' }).click();
await guard.getByText(/Ingreso registrado correctamente/).waitFor();
await shot(guard, '08-ingreso-registrado');
await guard.getByRole('button', { name: 'Registrar salida' }).click();
await guard.getByText(/Salida registrado correctamente/).waitFor();
ok('salida registrada', true);

// ── Sincronización al servidor
await guard.waitForTimeout(1200);
const logs = await guard.evaluate(async () => {
  const s = window.__superappSession;
  const r = await fetch('/api/v1/access-logs?limit=50', { headers: { Authorization: 'Bearer ' + s.token, 'X-Complex-ID': s.complexId } });
  return (await r.json()).data;
});
const mine = logs.filter((l) => l.visitor_name === 'María López');
ok('servidor: ENTRY y EXIT de María López', mine.some((l) => l.movement === 'ENTRY') && mine.some((l) => l.movement === 'EXIT'), `${mine.length} registros`);
ok('servidor: intento denegado del código inexistente', logs.some((l) => l.movement === 'ATTEMPT' && l.result === 'DENIED'));
ok('cola local vacía tras sync', (await guard.evaluate(() => JSON.parse(localStorage.getItem('superapp.accessLog.pending') || '[]').length)) === 0);

// ── Offline: registrar sin red y sincronizar al volver
await go(guard, '/porteria/movimiento');
await gctx.setOffline(true);
await guard.waitForTimeout(300);
await guard.getByPlaceholder('Juan Pérez').fill('Técnico Gas');
await guard.getByPlaceholder('Torre 3, Apto 502').fill('A-101');
await guard.getByRole('button', { name: 'Registrar ingreso' }).click();
await guard.getByText(/Ingreso registrado correctamente/).waitFor();
const pend = await guard.evaluate(() => JSON.parse(localStorage.getItem('superapp.accessLog.pending') || '[]').length);
ok('offline: registro queda en cola local', pend === 1, `pendientes=${pend}`);
await go(guard, '/').catch(() => {});
await gctx.setOffline(false);
await guard.waitForTimeout(300);
await go(guard, '/');
await guard.getByText('Operación en tiempo real').waitFor();
await guard.getByRole('button', { name: 'Sincronizar ahora' }).click().catch(() => {});
await guard.waitForTimeout(1500);
const after = await guard.evaluate(() => JSON.parse(localStorage.getItem('superapp.accessLog.pending') || '[]').length);
ok('online: la cola se sincroniza', after === 0, `pendientes=${after}`);
const logs2 = await guard.evaluate(async () => {
  const s = window.__superappSession;
  return (await (await fetch('/api/v1/access-logs?limit=50', { headers: { Authorization: 'Bearer ' + s.token, 'X-Complex-ID': s.complexId } })).json()).data;
});
ok('servidor recibió el registro offline (sin duplicar)', logs2.filter((l) => l.visitor_name === 'Técnico Gas').length === 1);

// ── Residente: revocar
// Recarga real (la sesión está en memoria, así que se vuelve a entrar) para leer la lista fresca del servidor.
await res.goto(APP + '/dev-login');
await res.getByRole('button', { name: /Ana García/ }).click();
await res.waitForSelector('.shell');
await go(res, '/autorizaciones');
await res.getByRole('heading', { name: 'Autorizaciones' }).waitFor();
ok('residente: pase usado salió de "Activas"', (await res.getByText('María López').count()) === 0);
await res.getByRole('tab', { name: /Historial/ }).click();
await res.getByText('María López').first().click();
await res.getByRole('heading', { name: 'Pase de acceso' }).waitFor();
ok('residente: pase usado ya no es revocable', (await res.getByRole('button', { name: 'Revocar' }).count()) === 0);

// ── ESCRITORIO: admin
const dctx = await browser.newContext({ serviceWorkers: 'block', viewport: { width: 1280, height: 800 } });
const admin = await login(dctx, 'Carlos Rojas');
await admin.getByText('Panel de administración').waitFor();
ok('admin: panel con accesos hoy', await admin.getByText('Accesos hoy').isVisible());
ok('desktop: sidebar visible, barra inferior oculta', (await admin.locator('.sidebar').isVisible()) && !(await admin.locator('.bottom-nav').isVisible()));
await shot(admin, '09-admin-panel');
await admin.getByRole('link', { name: 'Ver historial completo' }).click();
await admin.getByRole('heading', { name: 'Historial de accesos' }).waitFor();
await admin.locator('table tbody tr').first().waitFor();
const rows = await admin.locator('table tbody tr').count();
ok('historial: filas desde el servidor', rows >= 3, `${rows} filas`);
ok('historial: sin "Sin sincronizar"', (await admin.getByText('Sin sincronizar').count()) === 0);
await shot(admin, '10-historial');
await admin.locator('select[aria-label=Estado]').selectOption('DENIED');
const denied = await admin.locator('table tbody tr').count();
ok('historial: filtro Denegado', denied >= 1 && (await admin.locator('table tbody tr .badge.green').count()) === 0, `${denied} fila(s)`);

// ── RBAC en rutas
await go(res, '/historial');
await res.waitForTimeout(400);
ok('residente no accede a /historial (redirige)', new URL(res.url()).pathname === '/unauthorized');
await go(res, '/porteria/validar');
await res.waitForTimeout(400);
ok('residente no accede a /porteria/validar', new URL(res.url()).pathname === '/unauthorized');

ok('sin errores de consola/página', errors.length === 0, errors.slice(0, 3).join(' | '));
await browser.close();
console.log(fails ? `\n${fails} FALLAS` : '\nTODO OK');
process.exit(fails ? 1 : 0);
