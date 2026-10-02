// Circuit breaker del sistema QR: caída simulada, fallo rápido, fallback a PRE_AUTH y recuperación.
//
// Arrancar el ms de PRUEBAS con tiempos cortos (este script asume 300 ms de timeout y 4 s de enfriamiento):
//   QR_CALL_TIMEOUT_MS=300 QR_BREAKER_OPEN_SECONDS=4 QR_BREAKER_FAILURES=3 ENV=development ...
// Uso: BASE=http://localhost:8082/api/v1 node test/qr-breaker.mjs   (datos de prueba recién sembrados)
import crypto from 'node:crypto';

const B = process.env.BASE ?? 'http://localhost:8081/api/v1';
const TIMEOUT_MS = Number(process.env.QR_CALL_TIMEOUT_MS ?? 300);
const OPEN_S = Number(process.env.QR_BREAKER_OPEN_SECONDS ?? 4);
const C1 = '00000000-0000-0000-0000-000000000001';
const C2 = '00000000-0000-0000-0000-000000000002';
let fails = 0, total = 0;
const ok = (name, cond, extra = '') => { total++; if (!cond) fails++; console.log(`${cond ? 'PASS' : 'FAIL'}  ${name}${extra !== '' ? '  → ' + extra : ''}`); };
const section = (t) => console.log(`\n== ${t}`);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const timed = async (path, { token, method = 'GET', body } = {}) => {
  const t0 = performance.now();
  const r = await fetch(B + path, {
    method,
    headers: { ...(token && { Authorization: `Bearer ${token}` }), ...(body && { 'Content-Type': 'application/json' }) },
    body: body && JSON.stringify(body),
  });
  const j = await r.json().catch(() => null);
  return { s: r.status, d: j?.data, e: j?.error, ms: performance.now() - t0, retryAfter: r.headers.get('retry-after') };
};
const call = (...a) => timed(...a);

const people = (await call('/dev/users')).d;
const id = (n) => people.find((p) => p.name === n).user_id;
const T = async (name, complex) => (await call(`/dev/token?user_id=${id(name)}&complex_id=${complex}`)).d;
const ana = await T('Ana García', C1), pedro = await T('Pedro Gómez', C2), luis = await T('Luis Mora', C1), marta = await T('Marta Ruiz', C2);
const until = new Date(Date.now() + 4 * 3600e3).toISOString();
const makeQR = (who, name = 'Visita') => timed('/access-grants', { token: who.token, method: 'POST', body: { unit_id: who.unit_id, method: 'QR', visitor_type: 'VISITOR', visitor_name: name, valid_until: until } });
const makePre = (who, doc) => timed('/access-grants', { token: who.token, method: 'POST', body: { unit_id: who.unit_id, method: 'PRE_AUTH', visitor_type: 'VISITOR', visitor_name: 'Por documento', visitor_id_number: doc, valid_until: until } });
const validateQR = (who, code) => timed('/access-grants/validate', { token: who.token, method: 'POST', body: { complex_id: who.complex_id, method: 'QR', qr_code: code } });
const validatePre = (who, doc) => timed('/access-grants/validate', { token: who.token, method: 'POST', body: { complex_id: who.complex_id, method: 'PRE_AUTH', visitor_id_number: doc } });
const status = (who) => timed('/access/qr-system', { token: who.token });
const dev = () => timed('/dev/qr-system');
const setMode = (mode) => timed('/dev/qr-system', { method: 'PUT', body: { mode } });
const unitGrants = async (who) => (await call(`/units/${who.unit_id}/access-grants`, { token: who.token })).d.length;

await call('/dev/qr-system/reset', { method: 'POST' });
await setMode('UP');

section('Sistema QR operativo');
const s0 = await status(ana);
ok('estado público: disponible, circuito CLOSED', s0.s === 200 && s0.d.available === true && s0.d.state === 'CLOSED' && !s0.d.fallback, JSON.stringify(s0.d));
const q0 = await makeQR(ana, 'Antes de la caída');
ok('crea un QR → 201 con código de 256 bits', q0.s === 201 && /^[0-9a-f]{64}$/.test(q0.d.qr_code_hash ?? ''), `${Math.round(q0.ms)} ms`);
ok('el lector acepta el QR → válido (y se consume)', (await validateQR(luis, q0.d.qr_code_hash)).d?.valid === true);
const qKeep = await makeQR(ana, 'Pase creado antes de la caída'); // se valida después, tras la recuperación
ok('estado público sin token → 401', (await timed('/access/qr-system')).s === 401);
ok('el estado lo ven residentes y guardias', (await status(luis)).s === 200 && (await status(pedro)).s === 200);

section('Se cae el sistema QR (simulado: deja de responder)');
const d1 = await setMode('DOWN');
ok('dev: modo DOWN, circuito aún CLOSED', d1.d.mode === 'DOWN' && d1.d.state === 'CLOSED' && d1.d.simulated === true);
const grantsBefore = await unitGrants(ana);
const slow = [];
for (let i = 1; i <= 3; i++) slow.push(await makeQR(ana, `Durante la caída ${i}`));
ok('las 3 primeras peticiones NO se quedan colgadas: se cortan por timeout', slow.every((r) => r.ms >= TIMEOUT_MS * 0.8 && r.ms < TIMEOUT_MS * 6), slow.map((r) => Math.round(r.ms) + 'ms').join(', '));
ok('devuelven 503 controlado QR_001 con el fallback PRE_AUTH', slow.every((r) => r.s === 503 && r.e?.code === 'QR_001' && r.e.details?.fallback === 'PRE_AUTH' && r.e.details?.cause === 'TIMEOUT'), JSON.stringify(slow[0].e?.details));
ok('incluyen cabecera Retry-After', Number(slow[0].retryAfter) > 0, slow[0].retryAfter);
const s1 = await status(ana);
ok('el circuito se ABRE tras 3 fallos: no disponible, usar PRE_AUTH', s1.d.available === false && s1.d.state === 'OPEN' && s1.d.fallback === 'PRE_AUTH' && s1.d.retry_after_seconds > 0, JSON.stringify(s1.d));
const fast = await makeQR(ana, 'Con el circuito abierto');
ok('con el circuito abierto responde AL INSTANTE (sin esperar al dispositivo)', fast.s === 503 && fast.e.details.cause === 'CIRCUIT_OPEN' && fast.ms < 100, `${Math.round(fast.ms)} ms`);
const t0 = performance.now();
const burst = await Promise.all(Array.from({ length: 60 }, (_, i) => makeQR(i % 2 ? ana : pedro, `Ráfaga ${i}`)));
const burstMs = performance.now() - t0;
ok('60 peticiones simultáneas (2 conjuntos): todas 503 controlado y en conjunto rápido', burst.every((r) => r.s === 503 && r.e?.code === 'QR_001') && burstMs < 1000, `${Math.round(burstMs)} ms en total`);
ok('es un solo sistema: cae para todos los conjuntos', (await makeQR(pedro)).s === 503);
ok('no quedó ningún pase a medias', (await unitGrants(ana)) === grantsBefore, `${grantsBefore} → ${await unitGrants(ana)}`);

section('Lo que NO depende del QR sigue funcionando (pre-autorización)');
const p1 = await makePre(ana, 'COL600001');
ok('residente crea pre-autorización por documento → 201', p1.s === 201 && p1.d.method === 'PRE_AUTH' && p1.ms < 500, `${Math.round(p1.ms)} ms`);
const pv = await validatePre(luis, 'COL600001');
ok('guardia valida por documento → válido', pv.s === 200 && pv.d.valid === true && pv.ms < 500);
ok('guardia del otro conjunto no la usa (aislamiento intacto)', (await validatePre(marta, 'COL600001')).d?.valid === false);
const vq = await validateQR(luis, qKeep.d.qr_code_hash);
ok('validar por QR → 503 inmediato con fallback PRE_AUTH (el lector no responde)', vq.s === 503 && vq.e?.code === 'QR_001' && vq.e.details.fallback === 'PRE_AUTH' && vq.ms < 100, `${Math.round(vq.ms)} ms`);
ok('rutas legadas de QR también fallan rápido (qr-codes y lote)',
  (await timed('/access/qr-codes', { token: ana.token, method: 'POST', body: { unit_id: ana.unit_id, valid_until: until } })).s === 503 &&
  (await timed('/access/qr-codes/batch', { token: ana.token, method: 'POST', body: { unit_id: ana.unit_id, quantity: 3, valid_from: new Date().toISOString(), valid_until: until } })).s === 503);
ok('otras funciones no se ven afectadas (bitácora, entrada/salida de residentes)',
  (await timed('/access-logs', { token: luis.token, method: 'POST', body: { client_id: crypto.randomUUID(), movement: 'ENTRY', result: 'AUTHORIZED', visitor_name: 'Durante la caída', unit_label: 'A-101' } })).s === 201 &&
  (await timed('/resident-movements', { token: ana.token, method: 'POST', body: { client_id: crypto.randomUUID(), movement: 'ENTRY' } })).s === 201);

section('Recuperación (el circuito se cierra solo, con un intento de prueba)');
await setMode('UP');
const early = await makeQR(ana, 'Recién vuelto');
ok('aunque el sistema ya responde, el circuito sigue abierto hasta el enfriamiento (sin golpear al dispositivo)', early.s === 503 && early.e.details.cause === 'CIRCUIT_OPEN' && early.ms < 100);
const wait = ((await status(ana)).d.retry_after_seconds + 1) * 1000;
console.log(`   … esperando ${wait / 1000}s de enfriamiento`);
await sleep(wait);
const mid = await status(ana);
ok('pasado el enfriamiento el estado público invita a reintentar (HALF_OPEN, disponible)', mid.d.available === true && mid.d.state === 'HALF_OPEN', JSON.stringify(mid.d));
const probe = await makeQR(ana, 'Intento de prueba');
ok('pasado el enfriamiento, la siguiente petición es el intento de prueba y funciona → 201', probe.s === 201 && /^[0-9a-f]{64}$/.test(probe.d.qr_code_hash), `${probe.s}`);
const s2 = await status(ana);
ok('el circuito vuelve a CLOSED', s2.d.available === true && s2.d.state === 'CLOSED', JSON.stringify(s2.d));
ok('el QR creado antes de la caída ya se puede validar → válido', (await validateQR(luis, qKeep.d.qr_code_hash)).d?.valid === true);
ok('y el creado en el intento de prueba también', (await validateQR(luis, probe.d.qr_code_hash)).d?.valid === true);

section('Si el intento de prueba falla, vuelve a abrirse');
await setMode('DOWN');
for (let i = 0; i < 3; i++) await makeQR(ana, 'Otra caída');
ok('abierto otra vez tras 3 fallos', (await status(ana)).d.state === 'OPEN');
await sleep(((await status(ana)).d.retry_after_seconds + 1) * 1000);
const badProbe = await makeQR(ana, 'Prueba fallida');
ok('el intento de prueba falla (sigue caído) → 503 TIMEOUT', badProbe.s === 503 && badProbe.e.details.cause === 'TIMEOUT', `${Math.round(badProbe.ms)} ms`);
const s3 = await status(ana);
ok('queda OPEN con un nuevo enfriamiento', s3.d.state === 'OPEN' && s3.d.retry_after_seconds >= OPEN_S - 1, JSON.stringify(s3.d));
const rs = await timed('/dev/qr-system/reset', { method: 'POST' });
ok('dev: "cerrar circuito" lo cierra de inmediato', rs.d.state === 'CLOSED');
await setMode('UP');
ok('con todo restablecido vuelve a crear QR → 201', (await makeQR(ana, 'Todo normal')).s === 201);

section('Herramienta de simulación');
ok('modo inválido → 422', (await timed('/dev/qr-system', { method: 'PUT', body: { mode: 'ROTO' } })).s === 422);
const final = (await dev()).d;
ok('estado final: UP y CLOSED', final.mode === 'UP' && final.state === 'CLOSED');

console.log(`\n${total - fails}/${total} OK` + (fails ? `  —  ${fails} FALLAS` : ''));
process.exit(fails ? 1 : 0);
