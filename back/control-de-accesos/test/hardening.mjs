// Endurecimiento (revisión de código): robo de pases entre unidades, límite de cuerpo,
// formato único de errores, lote atómico.
// Uso: BASE=http://localhost:8082/api/v1 node test/hardening.mjs   (datos de prueba recién sembrados)
import crypto from 'node:crypto';

const B = process.env.BASE ?? 'http://localhost:8081/api/v1';
const C1 = '00000000-0000-0000-0000-000000000001';
const C2 = '00000000-0000-0000-0000-000000000002';
let fails = 0, total = 0;
const ok = (name, cond, extra = '') => { total++; if (!cond) fails++; console.log(`${cond ? 'PASS' : 'FAIL'}  ${name}${extra !== '' ? '  → ' + extra : ''}`); };
const section = (t) => console.log(`\n== ${t}`);

const call = async (path, { token, method = 'GET', body, raw } = {}) => {
  const r = await fetch(B + path, {
    method,
    headers: { ...(token && { Authorization: `Bearer ${token}` }), ...((body || raw) && { 'Content-Type': 'application/json' }) },
    body: raw ?? (body && JSON.stringify(body)),
  });
  const text = await r.text();
  let j = null; try { j = JSON.parse(text); } catch { /* no JSON */ }
  return { s: r.status, d: j?.data, e: j?.error, j, text };
};
const people = (await call('/dev/users')).d;
const id = (n) => people.find((p) => p.name === n).user_id;
const T = async (name, complex) => (await call(`/dev/token?user_id=${id(name)}&complex_id=${complex}`)).d;
const ana = await T('Ana García', C1), sofia = await T('Sofía Díaz', C1), pedro = await T('Pedro Gómez', C2);
const luis = await T('Luis Mora', C1), carlos = await T('Carlos Rojas', C1), marta = await T('Marta Ruiz', C2);
const until = new Date(Date.now() + 4 * 3600e3).toISOString();
const mkQR = (who, name) => call('/access-grants', { token: who.token, method: 'POST', body: { unit_id: who.unit_id, method: 'QR', visitor_type: 'VISITOR', visitor_name: name, valid_until: until } });
const mkPre = (who, name, doc) => call('/access-grants', { token: who.token, method: 'POST', body: { unit_id: who.unit_id, method: 'PRE_AUTH', visitor_type: 'VISITOR', visitor_name: name, visitor_id_number: doc, valid_until: until } });

section('C1 — un residente no puede ver (ni robar) los pases de otras unidades');
const anaQR = (await mkQR(ana, 'Visita secreta de Ana')).d;
const anaPre = (await mkPre(ana, 'Rappi de Ana', 'COL700700')).d;
const sofiaQR = (await mkQR(sofia, 'Visita de Sofía')).d;
const leaks = async (who) => {
  const bodies = await Promise.all(['/access/qr-codes', '/access/pre-auth', `/units/${who.unit_id}/access-grants`].map((p) => call(p, { token: who.token })));
  return bodies.map((b) => b.text).join('\n');
};
const sofiaSees = await leaks(sofia);
ok('Sofía (A-102) NO ve el código QR de Ana (A-101) en ningún listado', !sofiaSees.includes(anaQR.qr_code_hash));
ok('Sofía NO ve las cédulas de visitantes de Ana', !sofiaSees.includes('COL700700') && !sofiaSees.includes('COL123456'));
ok('Sofía NO ve ni los QR sembrados de otras unidades', !sofiaSees.includes('demo-qr-hash-0001'));
const sQR = (await call('/access/qr-codes', { token: sofia.token })).d;
ok('Sofía sí ve el suyo y solo el suyo', sQR.length === 1 && sQR[0].id === sofiaQR.id, `${sQR.length} pase(s)`);
const aQR = (await call('/access/qr-codes', { token: ana.token })).d;
ok('Ana ve los suyos (el creado y el sembrado de su unidad)', aQR.some((g) => g.id === anaQR.id) && aQR.every((g) => g.unit_id === ana.unit_id));
const aPre = (await call('/access/pre-auth', { token: ana.token })).d;
ok('Ana ve sus pre-autorizaciones', aPre.some((g) => g.id === anaPre.id));
const sPre = (await call('/access/pre-auth', { token: sofia.token })).d;
ok('Sofía no ve pre-autorizaciones ajenas', sPre.length === 0, `${sPre.length}`);
const attack = await call('/access-grants/validate', { token: luis.token, method: 'POST', body: { method: 'QR', qr_code: (sofiaSees.match(/"qr_code_hash":"([0-9a-f]{64})"/g) || []).map((m) => m.slice(16, 80)).find((h) => h !== sofiaQR.qr_code_hash) ?? 'x'.repeat(64) } });
ok('un QR ajeno no se puede obtener para usarlo (nada que probar con el guardia)', attack.d?.valid !== true);
const gQR = (await call('/access/qr-codes', { token: luis.token })).d;
ok('el guardia no recibe códigos QR de nadie (no es miembro de ninguna unidad)', gQR.length === 0 && !JSON.stringify(gQR).includes('qr_code_hash'));
const sum = await call('/access/summary', { token: carlos.token });
ok('el administrador consulta un RESUMEN (conteos), no los secretos', sum.s === 200 && sum.d.total >= 3 && typeof sum.d.active === 'number' && !sum.text.includes('qr_code_hash') && !sum.text.includes('COL'), JSON.stringify(sum.d));
ok('el guardia también puede ver el resumen', (await call('/access/summary', { token: luis.token })).s === 200);
ok('un residente NO puede pedir el resumen → 403', (await call('/access/summary', { token: ana.token })).s === 403);
ok('el resumen es solo de su conjunto (Marta ve otros totales)', (await call("/access/summary", { token: marta.token })).d?.total !== sum.d?.total);
const pSees = await leaks(pedro);
ok('Pedro (conjunto 2) no ve nada del conjunto 1', !pSees.includes(anaQR.qr_code_hash) && !pSees.includes('COL700700'));

section('C4 — límite de tamaño del cuerpo');
const big = await call('/access-logs', { token: luis.token, method: 'POST', raw: JSON.stringify({ client_id: crypto.randomUUID(), movement: 'ENTRY', result: 'AUTHORIZED', visitor_name: 'a', unit_label: 'b', vehicle: 'x'.repeat(2 * 1024 * 1024) }) });
ok('un cuerpo de 2 MB se rechaza con 413 (sin procesarlo)', big.s === 413, `${big.s}`);
const small = await call('/access-logs', { token: luis.token, method: 'POST', body: { client_id: crypto.randomUUID(), movement: 'ENTRY', result: 'AUTHORIZED', visitor_name: 'normal', unit_label: 'A-101' } });
ok('un cuerpo normal sigue funcionando', small.s === 201);
const big2 = await call('/access-grants', { token: ana.token, method: 'POST', raw: JSON.stringify({ unit_id: ana.unit_id, method: 'QR', visitor_type: 'VISITOR', visitor_name: 'y'.repeat(3 * 1024 * 1024), valid_until: until }) });
ok('lo mismo en otros endpoints (crear pase)', big2.s === 413, `${big2.s}`);

section('C6 — un solo formato de error');
const shape = (r) => r.j && r.j.success === false && typeof r.j.error?.code === 'string' && typeof r.j.error?.message === 'string';
const e403 = await call('/access-grants/validate', { token: ana.token, method: 'POST', body: { method: 'QR', qr_code: 'x' } });
ok('403 de rol (middleware) con {success,error:{code,message}}', e403.s === 403 && shape(e403), e403.text.slice(0, 120));
const e401 = await call('/access/qr-codes');
ok('401 sin token (middleware) con el mismo formato', e401.s === 401 && shape(e401), e401.text.slice(0, 120));
const e403b = await call('/access/summary', { token: ana.token });
ok('403 de personal con el mismo formato', e403b.s === 403 && shape(e403b));
const e400 = await call('/access-grants/no-es-uuid', { token: ana.token });
ok('400/404 de handler con el mismo formato', shape(e400), `${e400.s}`);

section('C5 — el lote de QR es atómico');
const before = (await call(`/units/${ana.unit_id}/access-grants`, { token: ana.token })).d.length;
const batch = await call('/access/qr-codes/batch', { token: ana.token, method: 'POST', body: { unit_id: ana.unit_id, quantity: 25, valid_from: new Date().toISOString(), valid_until: until } });
const after = (await call(`/units/${ana.unit_id}/access-grants`, { token: ana.token })).d.length;
ok('un lote de 25 crea los 25, todos con código distinto', batch.s === 201 && batch.d.length === 25 && new Set(batch.d.map((g) => g.qr_code_hash)).size === 25 && after - before === 25, `${batch.d?.length} creados`);
const badBatch = await call('/access/qr-codes/batch', { token: ana.token, method: 'POST', body: { unit_id: pedro.unit_id, quantity: 5, valid_from: new Date().toISOString(), valid_until: until } });
ok('un lote rechazado (unidad ajena) no deja nada', badBatch.s >= 400 && (await call(`/units/${ana.unit_id}/access-grants`, { token: ana.token })).d.length === after);

console.log(`\n${total - fails}/${total} OK` + (fails ? `  —  ${fails} FALLAS` : ''));
process.exit(fails ? 1 : 0);
