import { useCallback, useEffect, useMemo, useState } from "react";
import { api, formatWhen, hoursFromNow } from "./api.js";

const COMPLEX_ID = "00000000-0000-0000-0000-000000000001";

function actorOf(person) {
  return { complexId: COMPLEX_ID, userId: person.id, role: person.role, unitId: person.unit_id };
}

export function AccessControlModule() {
  const [people, setPeople] = useState([]);
  const [complexName, setComplexName] = useState("");
  const [userId, setUserId] = useState("");
  const [tab, setTab] = useState("personas");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  useEffect(() => {
    api.context()
      .then((data) => {
        setPeople(data.people);
        setComplexName(data.complexes[0]?.name || "");
        const resident = data.people.find((person) => person.role === "resident");
        setUserId(resident?.id || data.people[0]?.id || "");
      })
      .catch((err) => setError(err.message));
  }, []);

  const person = people.find((item) => item.id === userId);
  const actor = useMemo(
    () => (person ? actorOf(person) : null),
    [person],
  );

  const report = useCallback((err) => setError(err.message || "No se pudo completar la acción"), []);

  if (!actor) {
    return (
      <main className="shell">
        <h1>Control de accesos</h1>
        <p className="muted">{error || "Conectando con el módulo…"}</p>
      </main>
    );
  }

  return (
    <main className="shell">
      <header className="top">
        <div>
          <p className="eyebrow">Módulo independiente</p>
          <h1>Control de accesos</h1>
          <p className="muted">{complexName}</p>
        </div>
        <label className="actor">
          Probar como
          <select value={userId} onChange={(event) => { setUserId(event.target.value); setError(""); setNotice(""); }}>
            {people.map((item) => (
              <option key={item.id} value={item.id}>
                {item.full_name} · {item.role === "guard" ? "Portería" : item.unit_name}
              </option>
            ))}
          </select>
        </label>
      </header>

      <nav className="tabs">
        <button className={tab === "personas" ? "on" : ""} type="button" onClick={() => setTab("personas")}>Personas</button>
        <button className={tab === "vehiculos" ? "on" : ""} type="button" onClick={() => setTab("vehiculos")}>Vehículos</button>
      </nav>

      {error ? <p className="banner error">{error}</p> : null}
      {notice ? <p className="banner ok">{notice}</p> : null}

      {tab === "personas" ? (
        <PeoplePanel actor={actor} onError={report} onNotice={setNotice} clearError={() => setError("")} />
      ) : (
        <VehiclePanel actor={actor} onError={report} onNotice={setNotice} clearError={() => setError("")} />
      )}
    </main>
  );
}

function PeoplePanel({ actor, onError, onNotice, clearError }) {
  const [passes, setPasses] = useState([]);
  const [activity, setActivity] = useState(null);
  const [notes, setNotes] = useState([]);
  const [name, setName] = useState("");
  const [documentId, setDocumentId] = useState("");
  const [token, setToken] = useState("demo-token-maria-visitor");
  const [result, setResult] = useState(null);

  const reload = useCallback(async () => {
    if (actor.role === "resident" && actor.unitId) {
      setPasses(await api.listPasses(actor, actor.unitId));
      setNotes(await api.notifications(actor));
    } else {
      setActivity(await api.activity(actor));
    }
  }, [actor]);

  useEffect(() => {
    reload().catch(onError);
  }, [reload, onError]);

  async function createPass(event) {
    event.preventDefault();
    clearError();
    try {
      const created = await api.createPass(actor, {
        unit_id: actor.unitId,
        visitor_name: name,
        document: documentId,
        valid_until: hoursFromNow(8),
      });
      setName("");
      setDocumentId("");
      onNotice(`Pase creado. Token: ${created.token}`);
      await reload();
    } catch (error) {
      onError(error);
    }
  }

  async function run(action) {
    clearError();
    try {
      await action();
      await reload();
    } catch (error) {
      onError(error);
    }
  }

  if (actor.role === "guard") {
    return (
      <section className="grid">
        <form
          className="card"
          onSubmit={(event) => {
            event.preventDefault();
            run(async () => {
              const pass = await api.validatePass(actor, token);
              setResult(pass);
              onNotice(`${pass.visitor_name} ingresó.`);
            });
          }}
        >
          <h2>Ingreso de visitante</h2>
          <label>Token del QR<input value={token} onChange={(event) => setToken(event.target.value)} /></label>
          <div className="row">
            <button type="submit">Registrar ingreso</button>
            <button
              type="button"
              className="secondary"
              onClick={() => run(async () => {
                const pass = await api.checkOutPass(actor, token);
                setResult(pass);
                onNotice(`${pass.visitor_name} salió.`);
              })}
            >
              Registrar salida
            </button>
          </div>
          {result ? (
            <p className="result">
              {result.visitor_name} · {result.status}
              {result.document ? ` · doc ${result.document}` : ""}
            </p>
          ) : null}
        </form>
        <article className="card">
          <h2>Portería</h2>
          <ul className="stats">
            <li><strong>{activity?.currently_inside ?? 0}</strong> adentro</li>
            <li><strong>{activity?.entries_last_24h ?? 0}</strong> ingresos 24 h</li>
            <li><strong>{activity?.exits_last_24h ?? 0}</strong> salidas 24 h</li>
          </ul>
          <ul className="list">
            {(activity?.activities || []).map((item) => (
              <li key={`${item.pass_id}-${item.direction}-${item.occurred_at}`}>
                <span>{item.direction === "in" ? "Entrada" : "Salida"} · {item.unit_label}</span>
                <small>{formatWhen(item.occurred_at)}</small>
              </li>
            ))}
            {!activity?.activities?.length ? <li className="muted">Todavía no hay movimientos.</li> : null}
          </ul>
        </article>
      </section>
    );
  }

  return (
    <section className="grid">
      <form className="card" onSubmit={createPass}>
        <h2>Autorizar visitante · {actor.role === "resident" ? "tu unidad" : ""}</h2>
        <label>Nombre<input value={name} onChange={(event) => setName(event.target.value)} required /></label>
        <label>Documento<input value={documentId} onChange={(event) => setDocumentId(event.target.value)} /></label>
        <button type="submit">Crear pase (8 horas)</button>
      </form>
      <article className="card">
        <h2>Pases de la unidad</h2>
        <ul className="list">
          {passes.map((pass) => (
            <li key={pass.id}>
              <div>
                <strong>{pass.visitor_name}</strong>
                <small>{pass.status} · hasta {formatWhen(pass.valid_until)}</small>
                {pass.token ? <code>{pass.token}</code> : null}
              </div>
              {pass.status === "active" ? (
                <div className="row">
                  <button type="button" className="secondary" onClick={() => run(async () => {
                    const next = new Date(new Date(pass.valid_until).getTime() + 4 * 60 * 60 * 1000).toISOString();
                    await api.extendPass(actor, pass.id, next);
                    onNotice("Vigencia extendida 4 horas.");
                  })}>Extender</button>
                  <button type="button" className="danger" onClick={() => run(async () => {
                    await api.revokePass(actor, pass.id);
                    onNotice("Pase revocado.");
                  })}>Revocar</button>
                </div>
              ) : null}
            </li>
          ))}
          {!passes.length ? <li className="muted">No hay pases.</li> : null}
        </ul>
        {notes.length ? (
          <div className="notes">
            <h3>Avisos del anfitrión</h3>
            {notes.map((note) => <p key={note.id}>{note.body}</p>)}
          </div>
        ) : null}
      </article>
    </section>
  );
}

function VehiclePanel({ actor, onError, onNotice, clearError }) {
  const [preauths, setPreauths] = useState([]);
  const [logs, setLogs] = useState([]);
  const [lookup, setLookup] = useState(null);
  const [plate, setPlate] = useState("ABC-123");
  const [visitorName, setVisitorName] = useState("");

  const reload = useCallback(async () => {
    if (actor.role === "resident") setPreauths(await api.listPreauths(actor));
    else setLogs(await api.listAccess(actor));
  }, [actor]);

  useEffect(() => {
    reload().catch(onError);
  }, [reload, onError]);

  const knownPlate = useMemo(() => lookup?.visitor_preauth, [lookup]);

  if (actor.role === "guard") {
    return (
      <section className="grid">
        <form
          className="card"
          onSubmit={(event) => {
            event.preventDefault();
            clearError();
            api.lookupPlate(actor, plate).then(setLookup).catch(onError);
          }}
        >
          <h2>Consulta de placa</h2>
          <label>Placa<input value={plate} onChange={(event) => setPlate(event.target.value)} /></label>
          <button type="submit">Buscar</button>
          {lookup ? (
            <p className="result">
              {knownPlate
                ? `${lookup.plate}: ${knownPlate.visitor_name}, anfitrión ${knownPlate.host_name}, unidad ${knownPlate.unit_number}`
                : `${lookup.plate}: sin preautorización de visitante`}
            </p>
          ) : null}
          <div className="row">
            <button type="button" onClick={() => {
              clearError();
              api.logAccess(actor, { plate, direction: "in" })
                .then(async (entry) => {
                  onNotice(entry.visitor_preauth
                    ? `Entrada de ${entry.visitor_preauth.visitor_name}.`
                    : "Entrada registrada sin preautorización.");
                  await reload();
                })
                .catch(onError);
            }}>Entrada</button>
            <button type="button" className="secondary" onClick={() => {
              clearError();
              api.logAccess(actor, { plate, direction: "out" })
                .then(async () => {
                  onNotice("Salida registrada.");
                  await reload();
                })
                .catch(onError);
            }}>Salida</button>
          </div>
        </form>
        <article className="card">
          <h2>Bitácora</h2>
          <ul className="list">
            {logs.map((item) => (
              <li key={item.id}>
                <span>{item.direction === "in" ? "Entrada" : "Salida"} · {item.plate}</span>
                <small>
                  {item.visitor_preauth?.visitor_name || "Placa no preautorizada"} · {formatWhen(item.created_at)}
                </small>
              </li>
            ))}
            {!logs.length ? <li className="muted">Todavía no hay movimientos.</li> : null}
          </ul>
        </article>
      </section>
    );
  }

  return (
    <section className="grid">
      <form
        className="card"
        onSubmit={(event) => {
          event.preventDefault();
          clearError();
          api.createPreauth(actor, {
            unit_id: actor.unitId,
            plate,
            visitor_name: visitorName,
            valid_until: hoursFromNow(12),
          })
            .then(async () => {
              setVisitorName("");
              onNotice("Vehículo preautorizado por 12 horas.");
              await reload();
            })
            .catch(onError);
        }}
      >
        <h2>Preautorizar vehículo</h2>
        <label>Placa<input value={plate} onChange={(event) => setPlate(event.target.value)} required /></label>
        <label>Visitante<input value={visitorName} onChange={(event) => setVisitorName(event.target.value)} required /></label>
        <button type="submit">Autorizar</button>
      </form>
      <article className="card">
        <h2>Placas activas</h2>
        <ul className="list">
          {preauths.map((item) => (
            <li key={item.id}>
              <div>
                <strong>{item.plate}</strong>
                <small>{item.visitor_name} · hasta {formatWhen(item.valid_until)}</small>
              </div>
              <button type="button" className="danger" onClick={() => {
                clearError();
                api.revokePreauth(actor, item.id)
                  .then(async () => {
                    onNotice("Preautorización revocada.");
                    await reload();
                  })
                  .catch(onError);
              }}>Revocar</button>
            </li>
          ))}
          {!preauths.length ? <li className="muted">No hay placas activas.</li> : null}
        </ul>
      </article>
    </section>
  );
}
