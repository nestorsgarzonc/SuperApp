// Matriz de aislamiento multi-conjunto contra el ms (a través del proxy de Vite o directo).
import crypto from 'node:crypto';

const B = process.env.BASE ?? 'http://localhost:8081/api/v1';
const SECRET = process.env.JWT_SECRET; // mismo valor que usa el ms
if (!SECRET) { console.error('Set JWT_SECRET (same value the ms runs with)'); process.exit(1); }
const C1 = '00000000-0000-0000-0000-000000000001'; // Los Pinos
const C2 = '00000000-0000-0000-0000-000000000002'; // Alto del Bosque
let fails = 0, total = 0;
const ok = (name, cond, extra = '') => {
  total++;
  if (!cond) fails++;
  console.log(`${cond ? 'PASS' : 'FAIL'}  ${name}${extra !== '' ? '  → ' + extra : ''}`);
};
const section = (t) => console.log(`\n== ${t}`);

const call = async (path, { token, method = 'GET', body, headers = {} } = {}) => {
  const r = await fetch(B + path, {
    method,
    headers: { ...(token && { Authorization: `Bearer ${token}` }), ...(body && { 'Content-Type': 'application/json' }), ...headers },
    body: body && JSON.stringify(body),
  });
  const j = await r.json().catch(() => null);
  return { s: r.status, d: j?.data, e: j?.error, j };
};

// ── Personas (todas leídas de /dev/users, nada fijo)
const people = (await call('/dev/users')).d;
const byName = (n) => people.find((p) => p.name === n);
const id = (n) => byName(n).user_id;
const tok = async (name, complex, role) =>
  call(`/dev/token?user_id=${id(name)}&complex_id=${complex}${role ? '&role=' + role : ''}`);
const T = async (name, complex, role) => {
  const r = await tok(name, complex, role);
  if (r.s !== 200) throw new Error(`token ${name}@${complex}: ${r.s}`);
  return r.d;
};
// JWT forjado con el secreto (simula un token de otro emisor / manipulado)
const forge = (userId, role, complex) => {
  const b = (o) => Buffer.from(JSON.stringify(o)).toString('base64url');
  const m = b({ alg: 'HS256', typ: 'JWT' }) + '.' + b({ user_id: userId, email: 'x@x', role, complex_id: complex, iat: 0, exp: 4102444800 });
  return m + '.' + crypto.createHmac('sha256', SECRET).update(m).digest('base64url');
};

const ana1 = await T('Ana García', C1);
const pedro2 = await T('Pedro Gómez', C2);
const luis1 = await T('Luis Mora', C1);
const marta2 = await T('Marta Ruiz', C2);
const carlos1 = await T('Carlos Rojas', C1);
const sofia1 = await T('Sofía Díaz', C1);
const sofia2 = await T('Sofía Díaz', C2);
const andresR1 = await T('Andrés Vega', C1);
const andresG2 = await T('Andrés Vega', C2);
const val = (who, body, extraHeaders) => call('/access-grants/validate', { token: who.token, method: 'POST', body, headers: extraHeaders });
const qr = (who, code, extra) => val(who, { complex_id: who.complex_id, method: 'QR', qr_code: code }, extra);
const cedula = (who, n) => val(who, { complex_id: who.complex_id, method: 'PRE_AUTH', visitor_id_number: n });
const until = new Date(Date.now() + 4 * 3600e3).toISOString();

section('Dev: /dev/users y /dev/token salen de la base');
ok('7 personas sembradas', people.length >= 7, `${people.length}`);
ok('hay 2 conjuntos distintos', new Set(people.flatMap((p) => p.memberships.map((m) => m.complex_id))).size >= 2);
ok('Ana: RESIDENT @Los Pinos / unidad A-101', ana1.role === 'RESIDENT' && ana1.unit_name === 'A-101' && ana1.complex_name.includes('Pinos'));
ok('Pedro: RESIDENT @Alto del Bosque / B-201', pedro2.role === 'RESIDENT' && pedro2.unit_name === 'B-201' && pedro2.complex_name.includes('Bosque'));
ok('Luis: GUARD @1 · Marta: GUARD @2 · Carlos: ADMIN @1', luis1.role === 'GUARD' && marta2.role === 'GUARD' && carlos1.role === 'ADMIN');
ok('Sofía (en ambos): unidad A-102 @1 y B-202 @2', sofia1.unit_name === 'A-102' && sofia2.unit_name === 'B-202', `${sofia1.memberships.length} membresías`);
ok('Andrés: RESIDENT @1 pero GUARD @2', andresR1.role === 'RESIDENT' && andresG2.role === 'GUARD');
ok('Ana @conjunto 2 → 403 (no pertenece)', (await tok('Ana García', C2)).s === 403);
ok('Luis @conjunto 2 → 403 (guardia solo del 1)', (await tok('Luis Mora', C2)).s === 403);
ok('Luis pidiendo role=ADMIN @1 → 403 (no lo es)', (await tok('Luis Mora', C1, 'ADMIN')).s === 403);
ok('Marta @1 → 403', (await tok('Marta Ruiz', C1)).s === 403);
ok('usuario inexistente → 404', (await call(`/dev/token?user_id=${crypto.randomUUID()}&complex_id=${C1}`)).s === 404);
ok('sin parámetros → 422', (await call('/dev/token')).s === 422);

section('Validar QR: un guardia solo valida su conjunto');
const g1 = (await call('/access-grants', { token: ana1.token, method: 'POST', body: { unit_id: ana1.unit_id, method: 'QR', visitor_type: 'VISITOR', visitor_name: 'Visita de Ana', valid_until: until } })).d;
ok('Ana crea QR en su unidad (conjunto 1)', g1?.qr_code_hash, g1?.status);
const r1 = await qr(marta2, g1.qr_code_hash);
ok('Marta (guardia 2) valida QR del conjunto 1 → NO válido', r1.s === 200 && r1.d.valid === false, r1.d?.message);
const ghost = await qr(marta2, crypto.randomBytes(32).toString('hex'));
ok('NO se revela que el código existe en otro conjunto: la respuesta es idéntica a la de un código inexistente', JSON.stringify(r1.d) === JSON.stringify(ghost.d) && r1.d.reason === 'NOT_FOUND' && !r1.d.access_grant && !r1.d.unit_id && !r1.d.visitor_name && !/complex|conjunto/i.test(JSON.stringify(r1.d)), JSON.stringify(r1.d));
const r2 = await qr(luis1, 'demo-qr-hash-c2-0001');
ok('Luis (guardia 1) valida QR del conjunto 2 → NO válido', r2.s === 200 && r2.d.valid === false, r2.d?.message);
const r3 = await qr(marta2, 'demo-qr-hash-c2-0001');
ok('…y el QR del conjunto 2 NO se consumió por el intento de Luis: Marta lo valida OK', r3.d?.valid === true);
const r4 = await qr(luis1, g1.qr_code_hash);
ok('…ni el del conjunto 1 por el intento de Marta: Luis lo valida OK', r4.d?.valid === true);
ok('segunda validación → ya usado', (await qr(luis1, g1.qr_code_hash)).d?.valid === false);

section('Forzar otro conjunto con header/body: se ignora (manda el token)');
const fresh2 = (await call('/access-grants', { token: pedro2.token, method: 'POST', body: { unit_id: pedro2.unit_id, method: 'QR', visitor_type: 'VISITOR', visitor_name: 'Visita de Pedro', valid_until: until } })).d;
const h1 = await qr(luis1, fresh2.qr_code_hash, { 'X-Complex-ID': C2 });
ok('Luis + header X-Complex-ID=conjunto 2 → sigue siendo conjunto 1 (QR ajeno no válido)', h1.s === 200 && h1.d.valid === false, h1.d?.message);
const h2 = await val(luis1, { complex_id: C2, method: 'QR', qr_code: fresh2.qr_code_hash });
ok('Luis + body complex_id=conjunto 2 → ignorado', h2.s === 200 && h2.d.valid === false);
const h3 = await call(`/access-logs?complex_id=${C2}`, { token: luis1.token });
ok('Luis lista access-logs con ?complex_id=2 → solo ve el suyo (1)', h3.s === 200 && h3.d.every((l) => l.complex_id === C1));

section('Pre-autorización por cédula: aislada por conjunto');
const p1 = await cedula(luis1, 'COL777777');
ok('Luis valida cédula del conjunto 2 (COL777777) → NO válido', p1.d?.valid === false, p1.d?.message);
const p2 = await cedula(marta2, 'COL123456');
const ghostDoc = await cedula(marta2, 'COL000000');
ok('Marta valida cédula del conjunto 1 (COL123456) → NO válido, igual que una cédula inexistente', p2.d?.valid === false && JSON.stringify(p2.d) === JSON.stringify(ghostDoc.d), p2.d?.message);
const p3 = await cedula(marta2, 'COL777777');
ok('Marta valida COL777777 → válido, unidad B-201', p3.d?.valid === true && p3.d.access_grant?.complex_id === C2 && p3.d.unit_name === 'B-201', p3.d?.unit_name);
const p4 = await cedula(luis1, 'COL123456');
ok('Luis valida COL123456 → válido y el servidor devuelve el nombre de la unidad', p4.d?.valid === true && p4.d.unit_name === 'A-101', p4.d?.unit_name);
const d1 = await cedula(luis1, 'COL555555');
ok('misma cédula en ambos conjuntos: Luis consume la del 1', d1.d?.valid === true && d1.d.access_grant.complex_id === C1 && d1.d.access_grant.visitor_name === 'Misma Cédula Pinos');
const d2 = await cedula(marta2, 'COL555555');
ok('…y la del conjunto 2 sigue disponible para Marta', d2.d?.valid === true && d2.d.access_grant.complex_id === C2 && d2.d.access_grant.visitor_name === 'Misma Cédula Bosque');
ok('…cada una de un solo uso (Luis otra vez → no válido)', (await cedula(luis1, 'COL555555')).d?.valid === false);
const noDoc = await call('/access-grants', { token: ana1.token, method: 'POST', body: { unit_id: ana1.unit_id, method: 'PRE_AUTH', visitor_type: 'VISITOR', visitor_name: 'Sin cédula', valid_until: until } });
ok('crear PRE_AUTH sin cédula → 422', noDoc.s === 422, JSON.stringify(noDoc.e?.details?.field_errors));
const pa = await call('/access-grants', { token: ana1.token, method: 'POST', body: { unit_id: ana1.unit_id, method: 'PRE_AUTH', visitor_type: 'DELIVERY', visitor_name: 'Rappi Ana', visitor_id_number: 'COL999001', valid_until: until } });
ok('Ana crea PRE_AUTH con cédula → 201', pa.s === 201 && pa.d.method === 'PRE_AUTH');
ok('Marta (guardia 2) no puede usar la cédula de Ana', (await cedula(marta2, 'COL999001')).d?.valid === false);
const pv = await cedula(luis1, 'COL999001');
ok('Luis sí: válido y trae el nombre y la unidad', pv.d?.valid === true && pv.d.visitor_name === 'Rappi Ana' && pv.d.unit_id === ana1.unit_id);

section('Roles: quién NO puede validar');
ok('Ana (residente) → 403', (await qr(ana1, 'x')).s === 403);
ok('Pedro (residente) → 403', (await qr(pedro2, 'x')).s === 403);
ok('Sofía @1 (residente en ambos) → 403', (await qr(sofia1, 'x')).s === 403);
ok('Sofía @2 (residente en ambos) → 403', (await qr(sofia2, 'x')).s === 403);
ok('Andrés como RESIDENTE @1 → 403 aunque sea guardia del 2', (await qr(andresR1, 'x')).s === 403);
const a1 = await qr(andresG2, 'demo-qr-hash-0001');
ok('Andrés como GUARDIA @2 valida QR del conjunto 1 → NO válido', a1.s === 200 && a1.d.valid === false, a1.d?.message);
const fresh2b = (await call('/access-grants', { token: pedro2.token, method: 'POST', body: { unit_id: pedro2.unit_id, method: 'QR', visitor_type: 'VISITOR', visitor_name: 'Otra de Pedro', valid_until: until } })).d;
const a2 = await qr(andresG2, fresh2b.qr_code_hash, { 'X-Complex-ID': C1 });
ok('Andrés GUARDIA @2 + header conjunto 1 → sigue en el 2 → valida el QR de Pedro', a2.d?.valid === true);
ok('Carlos (admin 1) valida en el 1', (await cedula(carlos1, 'COL123456')).s === 200);
ok('sin token → 401', (await call('/access-grants/validate', { method: 'POST', body: { method: 'QR', qr_code: 'x' } })).s === 401);

section('Tokens manipulados (rol/conjunto que no corresponde)');
const forgedGuard = forge(id('Ana García'), 'GUARD', C1);
const f1 = await call('/access-grants/validate', { token: forgedGuard, method: 'POST', body: { method: 'QR', qr_code: 'demo-qr-hash-0001' } });
ok('token con role=GUARD para Ana (solo residente) → 403', f1.s === 403, f1.e ?? JSON.stringify(f1.j));
const forgedOther = forge(id('Luis Mora'), 'GUARD', C2);
const f2 = await call('/access-grants/validate', { token: forgedOther, method: 'POST', body: { method: 'QR', qr_code: 'x' } });
ok('token de Luis con complex_id=2 → 403 (no es de ese conjunto)', f2.s === 403, JSON.stringify(f2.j));
const forgedAdmin = forge(id('Luis Mora'), 'ADMIN', C1);
const f3 = await call('/access-logs', { token: forgedAdmin });
ok('token de Luis con role=ADMIN → 403 (es guardia, no admin)', f3.s === 403);

section('Crear/leer autorizaciones: aislamiento entre conjuntos y unidades');
const x1 = await call('/access-grants', { token: ana1.token, method: 'POST', body: { unit_id: pedro2.unit_id, method: 'QR', visitor_type: 'VISITOR', visitor_name: 'Intruso', valid_until: until } });
ok('Ana crea grant en la unidad de Pedro (conjunto 2) → rechazado', [403, 422].includes(x1.s), `${x1.s}`);
const x2 = await call('/access-grants', { token: ana1.token, method: 'POST', body: { unit_id: '10000000-0000-0000-0000-000000000102', method: 'QR', visitor_type: 'VISITOR', visitor_name: 'Misma torre, otra unidad', valid_until: until } });
ok('Ana crea grant en A-102 (mismo conjunto, otra unidad) → 403', x2.s === 403, `${x2.s}`);
const s1 = await call('/access-grants', { token: sofia1.token, method: 'POST', body: { unit_id: sofia1.unit_id, method: 'QR', visitor_type: 'VISITOR', visitor_name: 'Sofía→1', valid_until: until } });
ok('Sofía con token @1 crea en su unidad A-102 → 201', s1.s === 201);
const s2 = await call('/access-grants', { token: sofia1.token, method: 'POST', body: { unit_id: sofia2.unit_id, method: 'QR', visitor_type: 'VISITOR', visitor_name: 'Sofía→2 desde 1', valid_until: until } });
ok('Sofía con token @1 intenta dar acceso en B-202 (conjunto 2, también suya) → rechazado', [403, 422].includes(s2.s), `${s2.s}`);
const s3 = await call('/access-grants', { token: sofia2.token, method: 'POST', body: { unit_id: sofia2.unit_id, method: 'QR', visitor_type: 'VISITOR', visitor_name: 'Sofía→2', valid_until: until } });
ok('Sofía con token @2 en B-202 → 201', s3.s === 201);
const s4 = await call('/access-grants', { token: sofia2.token, method: 'POST', body: { unit_id: sofia1.unit_id, method: 'QR', visitor_type: 'VISITOR', visitor_name: 'Sofía→1 desde 2', valid_until: until } });
ok('Sofía con token @2 intenta A-102 (conjunto 1) → rechazado', [403, 422].includes(s4.s), `${s4.s}`);
ok('el grant de Sofía @1 NO es válido para el guardia del 2', (await qr(marta2, s1.d.qr_code_hash)).d?.valid === false);
ok('…pero sí para el guardia del 1', (await qr(luis1, s1.d.qr_code_hash)).d?.valid === true);
const rd = await call(`/access-grants/${s3.d.id}`, { token: sofia1.token });
ok('Sofía lee con token @1 un grant del conjunto 2 → bloqueado (cross-tenant)', [403, 404].includes(rd.s), `${rd.s}`);
const rd2 = await call(`/access-grants/${s3.d.id}`, { token: sofia2.token });
ok('…con token @2 sí lo lee', rd2.s === 200);
const ghostId = crypto.randomUUID();
const probeReal = await call(`/access-grants/${g1.id}`, { token: pedro2.token });
const probeGhost = await call(`/access-grants/${ghostId}`, { token: pedro2.token });
ok('leer por id un pase de otro conjunto es indistinguible de uno inexistente (404 en ambos, sin revelar que existe)', probeReal.s === 404 && probeGhost.s === 404 && probeReal.e?.code === probeGhost.e?.code && probeReal.e?.message.replace(g1.id, 'X') === probeGhost.e?.message.replace(ghostId, 'X'), `${probeReal.s}/${probeGhost.s}`);
const neighbor = await call(`/access-grants/${g1.id}`, { token: sofia1.token });
ok('…y también el pase de un vecino de OTRA unidad del mismo conjunto (Sofía A-102 lee uno de Ana A-101) → 404', neighbor.s === 404, `${neighbor.s}`);
ok('Pedro lista la unidad de Ana → 403', (await call(`/units/${ana1.unit_id}/access-grants`, { token: pedro2.token })).s === 403);
const cn = await call(`/access-grants/${fresh2b.id}/cancel`, { token: ana1.token, method: 'PATCH' });
ok('Ana cancela un grant de Pedro → 404 (igual que uno inexistente)', cn.s === 404 && (await call(`/access-grants/${crypto.randomUUID()}/cancel`, { token: ana1.token, method: 'PATCH' })).s === 404, `${cn.s}`);

section('Bitácora (access-logs) por conjunto');
const cid = crypto.randomUUID();
const log = (who, extra = {}) => call('/access-logs', { token: who.token, method: 'POST', body: { client_id: cid, movement: 'ENTRY', result: 'AUTHORIZED', visitor_name: 'Visita', unit_label: 'U', ...extra } });
const w1 = await log(luis1, { visitor_name: 'Entrada en Pinos' });
ok('Luis registra ingreso en el 1 → 201', w1.s === 201 && w1.d.complex_id === C1);
const w2 = await log(marta2, { visitor_name: 'Entrada en Bosque' });
ok('Marta usa el MISMO client_id en el 2 → 201 (es otro conjunto, no idempotencia)', w2.s === 201 && w2.d.id !== w1.d.id && w2.d.complex_id === C2);
const l1 = (await call('/access-logs?limit=500', { token: luis1.token })).d;
const l2 = (await call('/access-logs?limit=500', { token: marta2.token })).d;
ok('Luis solo ve registros del conjunto 1', l1.length > 0 && l1.every((l) => l.complex_id === C1) && l1.some((l) => l.visitor_name === 'Entrada en Pinos') && !l1.some((l) => l.visitor_name === 'Entrada en Bosque'));
ok('Marta solo ve registros del conjunto 2', l2.length > 0 && l2.every((l) => l.complex_id === C2) && l2.some((l) => l.visitor_name === 'Entrada en Bosque') && !l2.some((l) => l.visitor_name === 'Entrada en Pinos'));
const lc = (await call('/access-logs?limit=500', { token: carlos1.token })).d;
ok('Carlos (admin 1) ve el registro de Luis, no el de Marta', lc.some((l) => l.visitor_name === 'Entrada en Pinos') && !lc.some((l) => l.visitor_name === 'Entrada en Bosque'));
const wx = await log(luis1, { client_id: crypto.randomUUID(), grant_id: fresh2.id, visitor_name: 'Con grant ajeno' });
ok('grant_id de otro conjunto se descarta', wx.s === 201 && !wx.d.grant_id);
const wa = await call('/access-logs', { token: andresG2.token, method: 'POST', body: { client_id: crypto.randomUUID(), movement: 'EXIT', result: 'AUTHORIZED', visitor_name: 'Salida Andrés', unit_label: 'B-201' } });
ok('Andrés GUARDIA @2 registra → queda en el conjunto 2', wa.s === 201 && wa.d.complex_id === C2);
ok('Andrés RESIDENTE @1 no puede registrar → 403', (await call('/access-logs', { token: andresR1.token, method: 'POST', body: { client_id: crypto.randomUUID(), movement: 'ENTRY', result: 'AUTHORIZED', visitor_name: 'x', unit_label: 'y' } })).s === 403);
ok('Ana / Sofía (residentes) no listan access-logs → 403', (await call('/access-logs', { token: ana1.token })).s === 403 && (await call('/access-logs', { token: sofia2.token })).s === 403);
const l1b = (await call('/access-logs?limit=500', { token: luis1.token })).d;
ok('Luis no ve la salida de Andrés (conjunto 2)', !l1b.some((l) => l.visitor_name === 'Salida Andrés'));

console.log(`\n${total - fails}/${total} OK` + (fails ? `  —  ${fails} FALLAS` : ''));
process.exit(fails ? 1 : 0);
