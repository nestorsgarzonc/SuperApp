// Entrada/salida de residentes al edificio: estado, aislamiento por conjunto, presencia, concurrencia.
// Uso: BASE=http://localhost:8081/api/v1 node test/residents.mjs   (con datos de prueba recién sembrados)
import crypto from 'node:crypto';

const B = process.env.BASE ?? 'http://localhost:8081/api/v1';
const C1 = '00000000-0000-0000-0000-000000000001'; // Los Pinos
const C2 = '00000000-0000-0000-0000-000000000002'; // Alto del Bosque
let fails = 0, total = 0;
const ok = (name, cond, extra = '') => {
  total++;
  if (!cond) fails++;
  console.log(`${cond ? 'PASS' : 'FAIL'}  ${name}${extra !== '' ? '  → ' + extra : ''}`);
};
const section = (t) => console.log(`\n== ${t}`);

const call = async (path, { token, method = 'GET', body } = {}) => {
  const r = await fetch(B + path, {
    method,
    headers: { ...(token && { Authorization: `Bearer ${token}` }), ...(body && { 'Content-Type': 'application/json' }) },
    body: body && JSON.stringify(body),
  });
  const j = await r.json().catch(() => null);
  return { s: r.status, d: j?.data, e: j?.error, j };
};

const people = (await call('/dev/users')).d;
const id = (n) => people.find((p) => p.name === n).user_id;
const T = async (name, complex, role) => {
  const r = await call(`/dev/token?user_id=${id(name)}&complex_id=${complex}${role ? '&role=' + role : ''}`);
  if (r.s !== 200) throw new Error(`token ${name}@${complex}: ${r.s}`);
  return r.d;
};
const move = (who, movement, extra = {}) =>
  call('/resident-movements', { token: who.token, method: 'POST', body: { client_id: crypto.randomUUID(), movement, ...extra } });
const me = (who) => call('/resident-movements/me', { token: who.token });
const presence = (who) => call('/resident-movements/presence', { token: who.token });

const ana1 = await T('Ana García', C1);
const pedro2 = await T('Pedro Gómez', C2);
const luis1 = await T('Luis Mora', C1);
const marta2 = await T('Marta Ruiz', C2);
const carlos1 = await T('Carlos Rojas', C1);
const sofia1 = await T('Sofía Díaz', C1);
const sofia2 = await T('Sofía Díaz', C2);
const andresR1 = await T('Andrés Vega', C1);
const andresG2 = await T('Andrés Vega', C2);

section('Estado inicial');
const s0 = await me(ana1);
ok('Ana sin movimientos → UNKNOWN y lista vacía', s0.s === 200 && s0.d.status === 'UNKNOWN' && s0.d.movements.length === 0);
ok('presencia vacía al inicio', (await presence(luis1)).d?.length === 0);

section('Entrar y salir (máquina de estados)');
const e1 = await move(ana1, 'ENTRY');
ok('Ana ingresa al edificio → 201', e1.s === 201 && e1.d.movement === 'ENTRY' && e1.d.subject_type === 'RESIDENT', e1.s);
ok('el servidor pone su nombre y su unidad (no el cliente)', e1.d.visitor_name === 'Ana García' && e1.d.unit_label === 'A-101' && e1.d.complex_id === C1);
ok('registrado por ella misma, hora del servidor', e1.d.registered_by === ana1.user_id && Math.abs(Date.now() - new Date(e1.d.occurred_at)) < 5000);
const s1 = await me(ana1);
ok('estado → INSIDE con "desde"', s1.d.status === 'INSIDE' && !!s1.d.since);
const e2 = await move(ana1, 'ENTRY');
ok('ingresar estando dentro → 409', e2.s === 409, e2.e?.message);
const x1 = await move(ana1, 'EXIT');
ok('Ana sale del edificio → 201', x1.s === 201 && x1.d.movement === 'EXIT');
ok('estado → OUTSIDE', (await me(ana1)).d.status === 'OUTSIDE');
const x2 = await move(ana1, 'EXIT');
ok('salir estando fuera → 409', x2.s === 409, x2.e?.message);
const e3 = await move(ana1, 'ENTRY');
ok('vuelve a entrar → 201', e3.s === 201);
const h = (await me(ana1)).d;
ok('historial en orden: ENTRY, EXIT, ENTRY (más reciente primero)', h.movements.map((m) => m.movement).join() === 'ENTRY,EXIT,ENTRY' && h.status === 'INSIDE');

section('Reintentos y entradas inválidas');
const cid = crypto.randomUUID();
const r1 = await call('/resident-movements', { token: pedro2.token, method: 'POST', body: { client_id: cid, movement: 'ENTRY' } });
const r2 = await call('/resident-movements', { token: pedro2.token, method: 'POST', body: { client_id: cid, movement: 'ENTRY' } });
ok('mismo client_id repetido → 200 y el mismo registro (no 409)', r1.s === 201 && r2.s === 200 && r2.d.id === r1.d.id, `${r1.s} → ${r2.s}`);
ok('…y no duplica: Pedro tiene 1 movimiento', (await me(pedro2)).d.movements.length === 1);
ok('movement inválido → 422', (await move(ana1, 'ATTEMPT')).s === 422);
ok('sin client_id → 422', (await call('/resident-movements', { token: ana1.token, method: 'POST', body: { movement: 'EXIT' } })).s === 422);
ok('sin token → 401', (await call('/resident-movements', { method: 'POST', body: {} })).s === 401);
const spoof = await move(pedro2, 'EXIT', { visitor_name: 'Falso Nombre', unit_label: 'Z-999', result: 'DENIED', subject_type: 'VISITOR', occurred_at: '2020-01-01T00:00:00Z', registered_by: id('Ana García'), complex_id: C1 });
ok('el cliente no puede falsear nombre/unidad/hora/conjunto', spoof.s === 201 && spoof.d.visitor_name === 'Pedro Gómez' && spoof.d.unit_label === 'B-201' && spoof.d.complex_id === C2 && spoof.d.subject_type === 'RESIDENT' && spoof.d.registered_by === pedro2.user_id && new Date(spoof.d.occurred_at).getFullYear() >= 2026);

section('Concurrencia: toques simultáneos');
const sofiaBefore = (await me(sofia1)).d.status;
const burst = await Promise.all(Array.from({ length: 12 }, () => move(sofia1, 'ENTRY')));
const created = burst.filter((r) => r.s === 201).length, conflicts = burst.filter((r) => r.s === 409).length;
ok('12 ENTRY a la vez: exactamente 1 se registra, el resto 409', sofiaBefore === 'UNKNOWN' && created === 1 && conflicts === 11, `201×${created}, 409×${conflicts}`);
ok('Sofía queda con un solo ENTRY', (await me(sofia1)).d.movements.length === 1);

section('Aislamiento entre conjuntos (el estado es por conjunto)');
ok('Ana (conjunto 1) no puede operar en el 2: no hay token', (await call(`/dev/token?user_id=${id('Ana García')}&complex_id=${C2}`)).s === 403);
const s2in = await move(sofia2, 'ENTRY');
ok('Sofía, aunque está dentro en Pinos, entra en Alto del Bosque → 201 (estado independiente)', s2in.s === 201 && s2in.d.complex_id === C2 && s2in.d.unit_label === 'B-202', s2in.d?.unit_label);
const sMe1 = (await me(sofia1)).d, sMe2 = (await me(sofia2)).d;
ok('cada conjunto muestra solo sus movimientos de Sofía', sMe1.movements.every((m) => m.complex_id === C1 && m.unit_label === 'A-102') && sMe2.movements.every((m) => m.complex_id === C2));
ok('Ana nunca ve movimientos de otra persona', (await me(ana1)).d.movements.every((m) => m.visitor_name === 'Ana García'));
const sx = await move(sofia2, 'EXIT');
ok('sale del 2 y sigue DENTRO en el 1', sx.s === 201 && (await me(sofia2)).d.status === 'OUTSIDE' && (await me(sofia1)).d.status === 'INSIDE');

section('Roles: quién puede marcar entrada/salida');
ok('guardia no marca como residente → 403', (await move(luis1, 'ENTRY')).s === 403);
ok('administrador no marca como residente → 403', (await move(carlos1, 'ENTRY')).s === 403);
ok('Andrés como GUARDIA @2 no marca → 403', (await move(andresG2, 'ENTRY')).s === 403);
const aR = await move(andresR1, 'ENTRY');
ok('Andrés como RESIDENTE @1 sí → 201 en el conjunto 1, unidad A-102', aR.s === 201 && aR.d.complex_id === C1 && aR.d.unit_label === 'A-102');
ok('residente no consulta presencia → 403', (await presence(ana1)).s === 403);
ok('guardia no usa /me → 403', (await me(luis1)).s === 403);

section('Presencia: quién está dentro (guardia/admin, por conjunto)');
const p1 = (await presence(luis1)).d;
const names1 = p1.map((p) => p.name).sort();
ok('Luis (conjunto 1) ve dentro a Ana, Sofía y Andrés', names1.join() === ['Ana García', 'Andrés Vega', 'Sofía Díaz'].join(), names1.join(', '));
ok('cada entrada trae unidad y desde cuándo', p1.every((p) => p.unit_label && p.since));
const p2 = (await presence(marta2)).d;
ok('Marta (conjunto 2) ve dentro solo a Pedro? (Pedro entró y salió) → nadie; Sofía salió', p2.length === 0, p2.map((p) => p.name).join(', '));
const pedroIn = await move(pedro2, 'ENTRY');
ok('Pedro entra de nuevo', pedroIn.s === 201);
const p2b = (await presence(marta2)).d;
ok('Marta ahora ve a Pedro, y a nadie del conjunto 1', p2b.length === 1 && p2b[0].name === 'Pedro Gómez');
ok('Luis NO ve a Pedro', !(await presence(luis1)).d.some((p) => p.name === 'Pedro Gómez'));
ok('Carlos (admin 1) ve lo mismo que Luis', (await presence(carlos1)).d.length === 3);
await move(ana1, 'EXIT');
ok('Ana sale → desaparece de la presencia', !(await presence(luis1)).d.some((p) => p.name === 'Ana García'));
ok('Andrés (como guardia @2) ve la presencia del conjunto 2, no la del 1', (await presence(andresG2)).d.every((p) => p.name === 'Pedro Gómez'));

section('Se ve en el historial de portería/administración');
const logs = (await call('/access-logs?limit=500', { token: luis1.token })).d;
const resLogs = logs.filter((l) => l.subject_type === 'RESIDENT');
ok('access-logs de Luis incluye movimientos de residentes con su tipo', resLogs.length >= 5 && resLogs.every((l) => l.complex_id === C1 && l.visitor_type === 'Residente'));
ok('…entradas y salidas de Ana', resLogs.filter((l) => l.visitor_name === 'Ana García').map((l) => l.movement).includes('EXIT'));
ok('…y no incluye al conjunto 2', !logs.some((l) => l.visitor_name === 'Pedro Gómez'));
const onlyRes = (await call('/access-logs?subject=RESIDENT&limit=500', { token: carlos1.token })).d;
ok('filtro subject=RESIDENT', onlyRes.length > 0 && onlyRes.every((l) => l.subject_type === 'RESIDENT'));
await call('/access-logs', { token: luis1.token, method: 'POST', body: { client_id: crypto.randomUUID(), movement: 'ENTRY', result: 'AUTHORIZED', visitor_name: 'Visita X', unit_label: 'A-101' } });
const onlyVis = (await call('/access-logs?subject=VISITOR&limit=500', { token: carlos1.token })).d;
ok('filtro subject=VISITOR (no mezcla residentes)', onlyVis.length > 0 && onlyVis.every((l) => l.subject_type === 'VISITOR'));
ok('subject inválido → 422', (await call('/access-logs?subject=ALIEN', { token: luis1.token })).s === 422);
ok('un residente no lee la bitácora general → 403', (await call('/access-logs', { token: ana1.token })).s === 403);

console.log(`\n${total - fails}/${total} OK` + (fails ? `  —  ${fails} FALLAS` : ''));
process.exit(fails ? 1 : 0);
