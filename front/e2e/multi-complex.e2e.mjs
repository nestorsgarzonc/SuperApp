// Requiere: ms de control-de-accesos (ENV=development, build -tags dev, db/seeds/001_demo.sql) y `npm run dev` arriba; Edge/Chrome instalado.
// Reiniciar los datos de prueba antes de correr (consume pases de un solo uso).
import { chromium } from 'playwright-core';
import path from 'node:path';
import { mkdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const OUT = fileURLToPath(new URL('./shots/', import.meta.url));
mkdirSync(OUT, { recursive: true });
const APP = process.env.APP_URL ?? 'http://localhost:5173';
const C1 = 'Conjunto Residencial Los Pinos', C2 = 'Conjunto Alto del Bosque';
let fails = 0, total = 0;
const ok = (n, c, x = '') => { total++; if (!c) fails++; console.log(`${c ? 'PASS' : 'FAIL'}  ${n}${x ? '  → ' + x : ''}`); };
const section = (t) => console.log(`\n== ${t}`);

const browser = await chromium.launch({ channel: process.env.E2E_CHANNEL ?? 'msedge', headless: true });
const errors = [];
const mk = async (viewport = { width: 390, height: 844 }) => {
  const ctx = await browser.newContext({ viewport, deviceScaleFactor: 1 });
  return ctx;
};
const open = async (ctx, tag) => {
  const p = await ctx.newPage();
  p.on('pageerror', (e) => errors.push(`${tag}: ${e.message}`));
  p.on('console', (m) => m.type() === 'error' && errors.push(`${tag}: ${m.text()}`));
  await p.goto(APP);
  await p.getByTestId('dev-login').waitFor();
  return p;
};
const card = (p, name) => p.getByTestId('dev-login').getByRole('button', { name: new RegExp(name) });
const chipText = async (p) => {
  const sel = p.locator('select.chip-select');
  if (await sel.count()) return (await sel.locator('option:checked').innerText()).trim();
  return (await p.locator('.topbar .chip').innerText()).trim();
};
const sess = (p) => p.evaluate(() => JSON.parse(localStorage.getItem('superapp.session')));
const api = (p, path, init = {}) => p.evaluate(async ([path, init]) => {
  const s = JSON.parse(localStorage.getItem('superapp.session'));
  const r = await fetch('/api/v1' + path, { ...init, headers: { Authorization: 'Bearer ' + s.token, 'Content-Type': 'application/json', ...(init.headers ?? {}) } });
  return { s: r.status, d: (await r.json().catch(() => null))?.data };
}, [path, init]);

section('Login de desarrollo: personas leídas de la base');
{
  const ctx = await mk();
  const p = await open(ctx, 'login');
  const names = await p.getByTestId('dev-login').locator('.profile-card b').allInnerTexts();
  ok('7 personas listadas desde /dev/users', names.length === 7, names.join(', '));
  ok('Andrés muestra sus dos conjuntos y roles', (await card(p, 'Andrés').innerText()).includes('Portería') && (await card(p, 'Andrés').innerText()).includes('Residente'));
  ok('Sofía muestra ambos conjuntos', (await card(p, 'Sofía').innerText()).includes('Pinos') && (await card(p, 'Sofía').innerText()).includes('Bosque'));
  await card(p, 'Sofía').click();
  await p.getByRole('heading', { name: 'Selecciona tu conjunto' }).waitFor();
  const opts = await p.locator('.profile-card b').allInnerTexts();
  ok('"Selecciona tu conjunto" ofrece los 2 conjuntos', opts.includes(C1) && opts.includes(C2), opts.join(' | '));
  await p.screenshot({ path: path.join(OUT, '11-selecciona-conjunto.png') });
  await p.getByRole('button', { name: /Volver/ }).click();
  await p.getByTestId('dev-login').waitFor();
  ok('Volver regresa al listado', true);
  await ctx.close();
}

section('Sofía (residente en 1 y 2): un conjunto a la vez');
const sctx = await mk();
const sofia = await open(sctx, 'sofia');
await card(sofia, 'Sofía').click();
await sofia.locator('.profile-card', { hasText: C1 }).click();
await sofia.waitForSelector('.shell');
let ss = await sess(sofia);
ok('entra a Los Pinos como residente, unidad A-102', ss.complexName === C1 && ss.role === 'RESIDENT' && ss.unitName === 'A-102');
ok('el chip del encabezado es un selector de conjunto', (await sofia.locator('select.chip-select').count()) === 1, await chipText(sofia));
ok('el saludo usa el conjunto de la sesión (no uno fijo)', await sofia.locator('.page-head').getByText(new RegExp(C1)).isVisible());

// crea autorización por QR en el conjunto 1
await sofia.getByRole('link', { name: /Autorizar visitante/ }).click();
await sofia.getByPlaceholder('Juan Pérez').fill('Visita de Sofía en Pinos');
await sofia.locator('input[type=time]').nth(0).fill('00:00');
await sofia.locator('input[type=time]').nth(1).fill('23:59');
await sofia.getByRole('button', { name: 'Generar pase' }).click();
await sofia.getByRole('heading', { name: 'Pase de acceso' }).waitFor();
const codeP1 = (await sofia.locator('.pass .code').innerText()).replace('Código:', '').trim();
ok('pase QR del conjunto 1 creado', /^[0-9a-f]{64}$/.test(codeP1));
ok('el pase muestra el conjunto 1', await sofia.locator('.pass').getByText(C1).isVisible());

// cambia al conjunto 2 desde el selector
await sofia.locator('select.chip-select').selectOption({ label: new RegExp('x').source && (await sofia.locator('select.chip-select option').allInnerTexts()).find((t) => t.includes(C2)) });
await sofia.waitForFunction((n) => JSON.parse(localStorage.getItem('superapp.session')).complexName === n, C2);
ss = await sess(sofia);
ok('cambia a Alto del Bosque: otra unidad (B-202), mismo rol', ss.complexName === C2 && ss.unitName === 'B-202' && ss.role === 'RESIDENT');
ok('el token cambió de conjunto', ss.complexId.endsWith('0002'));
await sofia.getByRole('link', { name: /Mis autorizaciones/ }).click();
await sofia.getByRole('heading', { name: 'Autorizaciones' }).waitFor();
await sofia.waitForTimeout(500);
ok('en el conjunto 2 NO aparece el pase del conjunto 1', (await sofia.getByText('Visita de Sofía en Pinos').count()) === 0);
await sofia.getByRole('link', { name: /Nueva autorización/ }).click();
await sofia.getByPlaceholder('Juan Pérez').fill('Visita de Sofía en Bosque');
await sofia.locator('input[type=time]').nth(0).fill('00:00');
await sofia.locator('input[type=time]').nth(1).fill('23:59');
await sofia.getByRole('button', { name: 'Generar pase' }).click();
await sofia.getByRole('heading', { name: 'Pase de acceso' }).waitFor();
const codeP2 = (await sofia.locator('.pass .code').innerText()).replace('Código:', '').trim();
ok('pase QR del conjunto 2 creado (otro código)', /^[0-9a-f]{64}$/.test(codeP2) && codeP2 !== codeP1);
await sofia.screenshot({ path: path.join(OUT, '12-sofia-conjunto2.png') });
// lectura cruzada desde la API con el token del conjunto 2
const cross = await api(sofia, '/units/10000000-0000-0000-0000-000000000102/access-grants');
const g1 = (await api(sofia, '/units/10000000-0000-0000-0000-000000000202/access-grants')).d ?? [];
ok('con token @2, su unidad B-202 lista sus grants del 2', g1.some((g) => g.visitor_name === 'Visita de Sofía en Bosque'));
ok('el grant del conjunto 1 no es visible en B-202', !g1.some((g) => g.visitor_name === 'Visita de Sofía en Pinos'), `unidad A-102 con token @2 → ${cross.s} (unit-scoped por membresía)`);
// vuelve al 1 y vuelve a ver el suyo
await sofia.locator('select.chip-select').selectOption({ label: (await sofia.locator('select.chip-select option').allInnerTexts()).find((t) => t.includes(C1)) });
await sofia.waitForFunction((n) => JSON.parse(localStorage.getItem('superapp.session')).complexName === n, C1);
await sofia.goto(APP + '/autorizaciones');
await sofia.getByText('Visita de Sofía en Pinos').waitFor();
ok('de vuelta en Pinos ve su pase del conjunto 1', true);

section('Pre-autorización por documento (residente → portería)');
const ana = await open(await mk(), 'ana');
await card(ana, 'Ana García').click();
await ana.waitForSelector('.shell');
await ana.getByRole('link', { name: /Autorizar visitante/ }).click();
await ana.getByRole('radio', { name: 'Por documento' }).click();
await ana.getByPlaceholder('Juan Pérez').fill('Rappi de Ana');
await ana.locator('input[type=time]').nth(0).fill('00:00');
await ana.locator('input[type=time]').nth(1).fill('23:59');
await ana.getByRole('button', { name: 'Autorizar visitante' }).first().click();
ok('documento obligatorio para pre-autorización', await ana.getByText('El guardia validará con este documento').isVisible());
await ana.getByPlaceholder('1.020.345.678').fill('COL424242');
await ana.getByRole('button', { name: 'Autorizar visitante' }).first().click();
await ana.getByRole('heading', { name: 'Pase de acceso' }).waitFor();
ok('el pase de pre-autorización muestra el documento (sin QR)', (await ana.getByText('Documento: COL424242').count()) > 0 && (await ana.locator('.pass-qr img').count()) === 0);
await ana.screenshot({ path: path.join(OUT, '13-preauth-pase.png') });
await ana.goto(APP + '/autorizaciones');
await ana.getByText(/Por documento/).first().waitFor();
ok('en la lista figura "Por documento"', await ana.getByText(/Por documento/).first().isVisible());

// Guardia 2 (Marta) no puede usar la cédula ni el QR del conjunto 1; el guardia 1 (Luis) sí.
const marta = await open(await mk(), 'marta');
await card(marta, 'Marta Ruiz').click();
await marta.waitForSelector('.shell');
ok('Marta es guardia del conjunto 2', (await chipText(marta)).includes(C2));
await marta.getByRole('link', { name: /VALIDAR PASE/ }).click();
await marta.getByRole('tab', { name: /Documento/ }).click();
await marta.getByPlaceholder('1.020.345.678').fill('COL424242');
await marta.getByRole('button', { name: 'Validar pase' }).click();
await marta.getByText('✕ Pase no válido').waitFor();
ok('guardia 2 con la cédula del conjunto 1 → NO válido', true);
await marta.screenshot({ path: path.join(OUT, '14-marta-cedula-rechazada.png') });
await marta.getByRole('tab', { name: /Código QR/ }).click();
await marta.getByPlaceholder('SA-7K29-4QX1').fill(codeP1);
await marta.getByRole('button', { name: 'Validar pase' }).click();
await marta.getByText('✕ Pase no válido').waitFor();
ok('guardia 2 con el QR del conjunto 1 (de Sofía) → NO válido', true);
ok('cambiar de pestaña limpia el resultado anterior', (await marta.getByRole('tab', { name: /Documento/ }).click(), (await marta.getByText('✕ Pase no válido').count()) === 0));

const luis = await open(await mk(), 'luis');
await card(luis, 'Luis Mora').click();
await luis.waitForSelector('.shell');
ok('Luis es guardia del conjunto 1 (sin selector: un solo conjunto)', (await luis.locator('select.chip-select').count()) === 0 && (await chipText(luis)) === C1);
await luis.getByRole('link', { name: /VALIDAR PASE/ }).click();
await luis.getByRole('tab', { name: /Documento/ }).click();
await luis.getByPlaceholder('1.020.345.678').fill('COL424242');
await luis.getByRole('button', { name: 'Validar pase' }).click();
await luis.getByText('✓ Pase válido').waitFor();
ok('guardia 1 con la cédula de Ana → VÁLIDO (nombre y unidad)', (await luis.getByText('Rappi de Ana').count()) > 0 && (await luis.getByText('A-101').count()) > 0);
await luis.screenshot({ path: path.join(OUT, '15-luis-cedula-valida.png') });
await luis.getByRole('button', { name: 'Registrar ingreso' }).click();
await luis.getByRole('button', { name: 'Registrar ingreso' }).click();
await luis.getByText(/Ingreso registrado correctamente/).waitFor();
// segunda vez la misma cédula → ya usada
await luis.goto(APP + '/porteria/validar');
await luis.getByRole('tab', { name: /Documento/ }).click();
await luis.getByPlaceholder('1.020.345.678').fill('COL424242');
await luis.getByRole('button', { name: 'Validar pase' }).click();
await luis.getByText('✕ Pase no válido').waitFor();
ok('la misma cédula otra vez → no válida (un solo uso)', true);
// el QR de Sofía @1: Luis sí lo valida
await luis.getByRole('tab', { name: /Código QR/ }).click();
await luis.getByPlaceholder('SA-7K29-4QX1').fill(codeP1);
await luis.getByRole('button', { name: 'Validar pase' }).click();
await luis.getByText('✓ Pase válido').waitFor();
ok('guardia 1 con el QR de Sofía (conjunto 1) → VÁLIDO', (await luis.getByText('A-102').count()) > 0);

section('Andrés: residente en 1, guardia en 2 (mismo login, rol según conjunto)');
const andres = await open(await mk(), 'andres');
await card(andres, 'Andrés').click();
await andres.getByRole('heading', { name: 'Selecciona tu conjunto' }).waitFor();
await andres.locator('.profile-card', { hasText: C2 }).click();
await andres.waitForSelector('.shell');
ok('en Alto del Bosque entra como PORTERÍA', (await sess(andres)).role === 'GUARD' && (await andres.getByText('Operación en tiempo real').count()) > 0);
ok('ve las acciones de portería', (await andres.getByRole('link', { name: /VALIDAR PASE/ }).count()) === 1);
// el envío falla (servidor inalcanzable para POST /access-logs) aunque haya red para lo demás
await andres.goto(APP + '/porteria/movimiento');
let postsAfterSwitch = 0, switched = false;
await andres.route('**/api/v1/access-logs*', (r) => {
  if (r.request().method() === 'POST') { if (switched) postsAfterSwitch++; return r.abort(); }
  return r.continue();
});
await andres.getByPlaceholder('Juan Pérez').fill('Offline Bosque');
await andres.getByPlaceholder('Torre 3, Apto 502').fill('B-201');
await andres.getByRole('button', { name: 'Registrar ingreso' }).click();
await andres.getByText(/Ingreso registrado correctamente/).waitFor();
await andres.waitForTimeout(600);
const q1 = await andres.evaluate(() => JSON.parse(localStorage.getItem('superapp.accessLog.pending')));
ok('queda en cola local, atado al conjunto 2 y a Andrés', q1.length === 1 && q1[0].complexId.endsWith('0002') && q1[0].userId.endsWith('0007'));
// cambia a residente del conjunto 1 ANTES de que se sincronice
await andres.goto(APP + '/');
await andres.waitForSelector('select.chip-select');
switched = true;
await andres.locator('select.chip-select').selectOption({ label: (await andres.locator('select.chip-select option').allInnerTexts()).find((t) => t.includes(C1)) });
await andres.waitForFunction(() => JSON.parse(localStorage.getItem('superapp.session')).role === 'RESIDENT');
ok('en Pinos entra como RESIDENTE (sin portería)', (await andres.getByText('Hola, Andrés').count()) > 0 && (await andres.getByRole('link', { name: /VALIDAR PASE/ }).count()) === 0);
await andres.goto(APP + '/porteria/validar');
await andres.waitForTimeout(400);
ok('residente en Pinos no entra a /porteria/validar (aunque sea guardia en el 2)', new URL(andres.url()).pathname === '/');
await andres.evaluate(() => window.dispatchEvent(new Event('online')));
await andres.waitForTimeout(1200);
const q2 = await andres.evaluate(() => JSON.parse(localStorage.getItem('superapp.accessLog.pending')));
ok('el registro del conjunto 2 sigue en cola y NO se intentó enviar con la sesión del conjunto 1', q2.length === 1 && postsAfterSwitch === 0, `cola=${q2.length}, POST con sesión del 1=${postsAfterSwitch}`);
await andres.unroute('**/api/v1/access-logs*');
// vuelve al 2 → se sincroniza al conjunto 2
await andres.locator('select.chip-select').selectOption({ label: (await andres.locator('select.chip-select option').allInnerTexts()).find((t) => t.includes(C2)) });
await andres.waitForFunction(() => JSON.parse(localStorage.getItem('superapp.session')).role === 'GUARD');
await andres.waitForSelector('text=Operación en tiempo real');
await andres.getByRole('button', { name: 'Sincronizar ahora' }).click().catch(() => {});
await andres.waitForTimeout(1500);
const q3 = await andres.evaluate(() => JSON.parse(localStorage.getItem('superapp.accessLog.pending')));
ok('de vuelta en el conjunto 2 se sincroniza (cola vacía)', q3.length === 0);
const l2 = (await api(andres, '/access-logs?limit=100')).d;
ok('el registro quedó en el conjunto 2', l2.some((l) => l.visitor_name === 'Offline Bosque' && l.complex_id.endsWith('0002')));
const l1 = (await api(luis, '/access-logs?limit=100')).d;
ok('Luis (conjunto 1) NO lo ve', !l1.some((l) => l.visitor_name === 'Offline Bosque'));
ok('Luis sí ve sus registros del conjunto 1', l1.some((l) => l.visitor_name === 'Rappi de Ana'));

section('Admin (conjunto 1) no ve data del conjunto 2');
const dctx = await mk({ width: 1280, height: 800 });
const carlos = await open(dctx, 'carlos');
await card(carlos, 'Carlos Rojas').click();
await carlos.waitForSelector('.shell');
await carlos.goto(APP + '/historial');
await carlos.getByRole('heading', { name: 'Historial de accesos' }).waitFor();
await carlos.locator('select[aria-label=Fecha]').selectOption('all');
await carlos.waitForTimeout(800);
const rowsText = await carlos.locator('table tbody').innerText().catch(() => '');
ok('historial de Carlos incluye lo de Luis y no lo de Andrés/Bosque', rowsText.includes('Rappi de Ana') && !rowsText.includes('Offline Bosque'));
await carlos.screenshot({ path: path.join(OUT, '16-admin-historial-c1.png') });

ok('sin errores de consola/página (salvo 403/422 esperados)', errors.filter((e) => !/status of (403|422|401)|ERR_FAILED/.test(e)).length === 0, errors.filter((e) => !/status of (403|422|401)|ERR_FAILED/.test(e)).slice(0, 3).join(' | '));
await browser.close();
console.log(`\n${total - fails}/${total} OK` + (fails ? `  —  ${fails} FALLAS` : ''));
process.exit(fails ? 1 : 0);
