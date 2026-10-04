// Requiere: ms de control-de-accesos (ENV=development, build -tags dev, db/seeds/001_demo.sql) y `npm run dev` arriba; Edge/Chrome instalado.
// Reiniciar los datos de prueba antes de correr.
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
const open = async (name, viewport = { width: 390, height: 844 }) => {
  const ctx = await browser.newContext({ viewport });
  const p = await ctx.newPage();
  p.on('pageerror', (e) => errors.push(`${name}: ${e.message}`));
  p.on('console', (m) => m.type() === 'error' && errors.push(`${name}: ${m.text()}`));
  await p.goto(APP);
  await p.getByTestId('dev-login').waitFor();
  await p.getByTestId('dev-login').getByRole('button', { name: new RegExp(name) }).click();
  return p;
};
const status = (p) => p.getByTestId('building-status');
const goBuilding = async (p) => {
  await p.getByRole('link', { name: /Edificio/ }).last().click();
  await p.getByRole('heading', { name: 'Mi acceso al edificio' }).waitFor();
};
const inBtn = (p) => p.getByRole('button', { name: 'Ingresar al edificio' });
const outBtn = (p) => p.getByRole('button', { name: 'Salir del edificio' });

section('Residente: entrar y salir del edificio');
const ana = await open('Ana García');
await ana.waitForSelector('.shell');
ok('el residente ve "Entrar / salir del edificio" en su inicio', (await ana.getByRole('link', { name: /Entrar \/ salir del edificio/ }).count()) === 1);
await goBuilding(ana);
ok('estado inicial: sin movimientos', await status(ana).getByText('Aún no has registrado movimientos').isVisible());
ok('ambos botones habilitados (estado desconocido)', (await inBtn(ana).isEnabled()) && (await outBtn(ana).isEnabled()));
await ana.screenshot({ path: path.join(OUT, '20-edificio-inicial.png') });

await inBtn(ana).click();
await ana.getByText(/Ingreso registrado/).waitFor();
ok('ingresa: estado "dentro del edificio"', await status(ana).getByText('Estás dentro del edificio').isVisible());
ok('"Ingresar" se deshabilita, "Salir" queda disponible', !(await inBtn(ana).isEnabled()) && (await outBtn(ana).isEnabled()));
ok('muestra desde cuándo', await status(ana).getByText(/Desde Hoy/).isVisible());
ok('el movimiento aparece en "Mis movimientos"', (await ana.getByText('Ingreso al edificio').count()) >= 1 && (await ana.getByText(/A-101/).count()) >= 1);
await ana.screenshot({ path: path.join(OUT, '21-edificio-dentro.png') });

await outBtn(ana).click();
await ana.getByText(/Salida registrado/).waitFor();
ok('sale: estado "fuera del edificio"', await status(ana).getByText('Estás fuera del edificio').isVisible());
ok('"Salir" se deshabilita, "Ingresar" disponible', !(await outBtn(ana).isEnabled()) && (await inBtn(ana).isEnabled()));
ok('historial con 2 movimientos (salida arriba)', (await ana.locator('.list li').count()) === 2 && (await ana.locator('.list li').first().innerText()).includes('Salida del edificio'));
await ana.screenshot({ path: path.join(OUT, '22-edificio-fuera.png') });
await ana.reload();
await status(ana).getByText('Estás fuera del edificio').waitFor();
ok('al recargar, el estado se conserva (viene del servidor)', true);

// toque doble rápido: solo un movimiento
await inBtn(ana).dblclick();
await ana.getByText(/Ingreso registrado/).waitFor();
await ana.waitForTimeout(500);
const rows = await ana.locator('.list li').count();
ok('doble clic en "Ingresar" registra un solo movimiento', rows === 3, `${rows} movimientos`);

// fallo de red al registrar: error visible y se puede reintentar
await outBtn(ana).waitFor();
await ana.route('**/api/v1/resident-movements', (r) => (r.request().method() === 'POST' ? r.abort() : r.continue()));
await outBtn(ana).click();
await ana.getByText(/Sin conexión con el servidor/).waitFor();
ok('sin conexión: muestra error y NO cambia el estado', await status(ana).getByText('Estás dentro del edificio').isVisible());
await ana.unroute('**/api/v1/resident-movements');
await outBtn(ana).click();
await ana.getByText(/Salida registrado/).waitFor();
ok('al reintentar con red, se registra', await status(ana).getByText('Estás fuera del edificio').isVisible());
await ana.goto(APP + '/edificio');
await inBtn(ana).click();   // Ana queda DENTRO para las pruebas de portería
await ana.getByText(/Ingreso registrado/).waitFor();

section('Portería y administración ven quién está dentro');
const luis = await open('Luis Mora');
await luis.getByTestId('presence-card').waitFor();
await luis.getByTestId('presence-card').getByText('Ana García').waitFor();
const card = luis.getByTestId('presence-card');
ok('Luis (guardia 1) ve "Residentes dentro del edificio (1)" con Ana', (await card.innerText()).includes('(1)') && (await card.innerText()).includes('A-101'));
const recent = luis.locator('.card', { hasText: 'Accesos recientes' });
await recent.getByText('Ana García').waitFor({ timeout: 8000 }).catch(async () => console.log('DEBUG recientes:', JSON.stringify(await recent.innerText()), '| logs:', await luis.evaluate(() => localStorage.getItem('superapp.accessLog.pending'))));
ok('"Accesos recientes" de portería lista el ingreso de la residente', (await recent.innerText()).includes('Ingreso al edificio'));
await luis.screenshot({ path: path.join(OUT, '23-porteria-presencia.png') });

const carlos = await open('Carlos Rojas', { width: 1280, height: 800 });
await carlos.getByTestId('presence-card').getByText('Ana García').waitFor();
ok('Carlos (admin 1) también la ve', true);
await carlos.goto(APP + '/historial');
await carlos.locator('select[aria-label=Fecha]').selectOption('all');
await carlos.locator('table tbody tr').first().waitFor();
const t = await carlos.locator('table tbody').innerText();
ok('el historial muestra "Ingreso al edificio" / "Salida del edificio" de Ana como Residente', t.includes('Ingreso al edificio') && t.includes('Salida del edificio') && t.includes('Residente') && t.includes('Ana García'));
await carlos.screenshot({ path: path.join(OUT, '24-admin-historial-residentes.png') });
await carlos.locator('select[aria-label=Tipo]').selectOption('Residente');
ok('filtro Tipo = Residente', (await carlos.locator('table tbody tr').count()) >= 4);

const marta = await open('Marta Ruiz');
await marta.getByTestId('presence-card').waitFor();
await marta.waitForTimeout(500);
ok('Marta (guardia del conjunto 2) NO ve a Ana', (await marta.getByTestId('presence-card').getByText('Ana García').count()) === 0 && (await marta.getByTestId('presence-card').innerText()).includes('(0)'));

// Ana sale → desaparece de la presencia al actualizar
await ana.goto(APP + '/edificio');
await outBtn(ana).click();
await ana.getByText(/Salida registrado/).waitFor();
await luis.getByRole('button', { name: 'Actualizar' }).click();
await luis.getByText('No hay residentes dentro en este momento.').waitFor();
ok('Ana sale → Luis la ve desaparecer al actualizar', true);

section('Permisos de pantalla');
await luis.goto(APP + '/edificio');
await luis.waitForTimeout(400);
ok('el guardia no entra a /edificio (redirige)', new URL(luis.url()).pathname === '/');
ok('el residente no ve la tarjeta de presencia en su inicio', (await ana.goto(APP + '/'), await ana.waitForSelector('.shell'), (await ana.getByTestId('presence-card').count()) === 0));

section('Sofía (en ambos conjuntos): el estado es por conjunto');
const sofia = await open('Sofía');
await sofia.getByRole('heading', { name: 'Selecciona tu conjunto' }).waitFor();
await sofia.locator('.profile-card', { hasText: C1 }).click();
await sofia.waitForSelector('.shell');
await goBuilding(sofia);
await inBtn(sofia).click();
await sofia.getByText(/Ingreso registrado/).waitFor();
ok('Sofía entra en Los Pinos (unidad A-102)', (await status(sofia).getByText('Estás dentro del edificio').isVisible()) && (await sofia.getByText(/A-102/).count()) >= 1);
await sofia.locator('select.chip-select').selectOption({ label: (await sofia.locator('select.chip-select option').allInnerTexts()).find((x) => x.includes(C2)) });
await sofia.waitForFunction((n) => JSON.parse(localStorage.getItem('superapp.session')).complexName === n, C2);
await goBuilding(sofia);
await status(sofia).getByText('Aún no has registrado movimientos').waitFor();
ok('en Alto del Bosque aparece SIN movimientos (no hereda el estado del otro conjunto)', (await sofia.locator('.list li').count()) === 0);
await inBtn(sofia).click();
await sofia.getByText(/Ingreso registrado/).waitFor();
ok('entra también en Alto del Bosque (unidad B-202)', (await sofia.getByText(/B-202/).count()) >= 1);
await luis.getByRole('button', { name: 'Actualizar' }).click();
await luis.getByTestId('presence-card').getByText('Sofía Díaz').waitFor();
ok('Luis (conjunto 1) ve a Sofía con su unidad del conjunto 1', (await luis.getByTestId('presence-card').innerText()).includes('A-102'));
await marta.getByRole('button', { name: 'Actualizar' }).click();
await marta.getByTestId('presence-card').getByText('Sofía Díaz').waitFor();
ok('Marta (conjunto 2) ve a Sofía con su unidad del conjunto 2', (await marta.getByTestId('presence-card').innerText()).includes('B-202'));

const real = errors.filter((e) => !/status of (403|409|422|401)|ERR_FAILED/.test(e));
ok('sin errores de consola/página (salvo los esperados)', real.length === 0, real.slice(0, 3).join(' | '));
await browser.close();
console.log(`\n${total - fails}/${total} OK` + (fails ? `  —  ${fails} FALLAS` : ''));
process.exit(fails ? 1 : 0);
