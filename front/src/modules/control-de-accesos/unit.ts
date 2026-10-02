/** Nombre de unidad para mostrar; el backend lo aporta en /validate y en los grants. */
export const unitLabel = (id?: string, name?: string) => name ?? (id ? `Unidad ${id.slice(0, 8)}` : '—');
