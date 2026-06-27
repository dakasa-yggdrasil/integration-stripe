// Format a Unix epoch (seconds) into a terse pt-BR absolute timestamp for the
// reconciliation roster, plus a short relative form. These are config-grade
// facts (when a charge was created), never anything customer-identifying.

/** "27/06 14:32" — date + time, no year (the roster is recent charges). */
export function formatCreated(unixSeconds: number): string {
  if (!Number.isFinite(unixSeconds) || unixSeconds <= 0) return "—";
  const d = new Date(unixSeconds * 1000);
  return new Intl.DateTimeFormat("pt-BR", {
    day: "2-digit",
    month: "2-digit",
    hour: "2-digit",
    minute: "2-digit"
  }).format(d);
}

/** "27/06/2026" — full date, with year (subscription period-end can be far out). */
export function formatDate(unixSeconds: number): string {
  if (!Number.isFinite(unixSeconds) || unixSeconds <= 0) return "—";
  const d = new Date(unixSeconds * 1000);
  return new Intl.DateTimeFormat("pt-BR", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric"
  }).format(d);
}

/**
 * "em 12 d" / "em 3 h" / "há 2 d" — a signed relative hint for a future-or-past
 * instant (subscription period-end can be either), or "" when unknown.
 */
export function relativeWhen(unixSeconds: number): string {
  if (!Number.isFinite(unixSeconds) || unixSeconds <= 0) return "";
  const deltaSec = unixSeconds - Math.floor(Date.now() / 1000);
  const future = deltaSec >= 0;
  const abs = Math.abs(deltaSec);
  const prefix = future ? "em" : "há";
  if (abs < 60) return future ? "agora" : "agora";
  const min = Math.floor(abs / 60);
  if (min < 60) return `${prefix} ${min} min`;
  const hours = Math.floor(min / 60);
  if (hours < 24) return `${prefix} ${hours} h`;
  const days = Math.floor(hours / 24);
  return `${prefix} ${days} d`;
}

/** "há 7 min" / "há 2 h" / "há 3 d" — a relative hint, or "" when unknown. */
export function relativeCreated(unixSeconds: number): string {
  if (!Number.isFinite(unixSeconds) || unixSeconds <= 0) return "";
  const deltaSec = Math.max(0, Math.floor(Date.now() / 1000) - unixSeconds);
  if (deltaSec < 60) return "agora";
  const min = Math.floor(deltaSec / 60);
  if (min < 60) return `há ${min} min`;
  const hours = Math.floor(min / 60);
  if (hours < 24) return `há ${hours} h`;
  const days = Math.floor(hours / 24);
  return `há ${days} d`;
}
