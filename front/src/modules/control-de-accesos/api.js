function headers(actor) {
  return {
    "Content-Type": "application/json",
    "X-Complex-Id": actor.complexId,
    "X-User-Id": actor.userId,
  };
}

async function request(path, { method = "GET", actor, body } = {}) {
  const response = await fetch(path, {
    method,
    headers: actor ? headers(actor) : { "Content-Type": "application/json" },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (response.status === 204) return null;
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(data.error || `Error ${response.status}`);
  }
  return data;
}

export const api = {
  context: () => request("/integration/context"),
  notifications: (actor) => request("/integration/notifications", { actor }),
  listPasses: (actor, unitId) => request(`/visitor-passes/units/${unitId}`, { actor }),
  createPass: (actor, body) => request("/visitor-passes", { method: "POST", actor, body }),
  revokePass: (actor, id) => request(`/visitor-passes/${id}/revoke`, { method: "POST", actor, body: {} }),
  extendPass: (actor, id, validUntil) =>
    request(`/visitor-passes/${id}/extend`, { method: "POST", actor, body: { valid_until: validUntil } }),
  validatePass: (actor, token) => request("/visitor-passes/validate", { method: "POST", actor, body: { token } }),
  checkOutPass: (actor, token) => request("/visitor-passes/check-out", { method: "POST", actor, body: { token } }),
  activity: (actor) => request("/visitor-passes/checkpoint-activity", { actor }),
  listPreauths: (actor) => request("/vehicles/visitor-preauth", { actor }),
  createPreauth: (actor, body) => request("/vehicles/visitor-preauth", { method: "POST", actor, body }),
  revokePreauth: (actor, id) => request(`/vehicles/visitor-preauth/${id}`, { method: "DELETE", actor }),
  lookupPlate: (actor, plate) => request(`/vehicles/lookup?plate=${encodeURIComponent(plate)}`, { actor }),
  logAccess: (actor, body) => request("/vehicles/access-log", { method: "POST", actor, body }),
  listAccess: (actor) => request("/vehicles/access-log", { actor }),
};

export function hoursFromNow(hours) {
  return new Date(Date.now() + hours * 60 * 60 * 1000).toISOString();
}

export function formatWhen(value) {
  if (!value) return "—";
  return new Date(value).toLocaleString("es-CO", { dateStyle: "short", timeStyle: "short" });
}
