// Currency formatting. Stripe amounts arrive in the SMALLEST currency unit
// (cents for BRL/USD/EUR, whole units for zero-decimal currencies like JPY),
// so a raw integer like 4_820_000 BRL must render as "R$ 48.200,00". We divide
// by the currency's minor-unit factor and format with Intl, rounding properly.

// Zero-decimal currencies (Stripe's list) — the amount IS the major unit, no
// division. Three-decimal currencies (BHD/KWD/OMR/TND) use 1000.
const ZERO_DECIMAL = new Set([
  "bif", "clp", "djf", "gnf", "jpy", "kmf", "krw", "mga", "pyg", "rwf",
  "ugx", "vnd", "vuv", "xaf", "xof", "xpf"
]);
const THREE_DECIMAL = new Set(["bhd", "jod", "kwd", "omr", "tnd"]);

/** Minor-unit decimal places for a currency (2 by default). */
export function currencyDecimals(currency: string): number {
  const c = currency.trim().toLowerCase();
  if (ZERO_DECIMAL.has(c)) return 0;
  if (THREE_DECIMAL.has(c)) return 3;
  return 2;
}

/**
 * Format a Stripe smallest-unit amount to its currency, e.g.
 * `formatMoney(4_820_000, "brl")` → "R$ 48.200,00". Uses the pt-BR locale so the
 * grouping/decimal separators match the rest of the console. Falls back to an
 * uppercase-code prefix if Intl doesn't know the currency.
 */
export function formatMoney(amountMinor: number, currency: string): string {
  const code = currency.trim().toUpperCase();
  const decimals = currencyDecimals(currency);
  const major = amountMinor / Math.pow(10, decimals);
  try {
    return new Intl.NumberFormat("pt-BR", {
      style: "currency",
      currency: code,
      minimumFractionDigits: decimals,
      maximumFractionDigits: decimals
    }).format(major);
  } catch {
    // Unknown currency code: format the number and prefix the raw code.
    const n = new Intl.NumberFormat("pt-BR", {
      minimumFractionDigits: decimals,
      maximumFractionDigits: decimals
    }).format(major);
    return `${code} ${n}`;
  }
}

/**
 * A compact form for KPI tiles: "R$ 48,2k" for large amounts, full otherwise.
 * Keeps the headline terse without losing the currency symbol.
 */
export function formatMoneyCompact(amountMinor: number, currency: string): string {
  const code = currency.trim().toUpperCase();
  const decimals = currencyDecimals(currency);
  const major = amountMinor / Math.pow(10, decimals);
  try {
    return new Intl.NumberFormat("pt-BR", {
      style: "currency",
      currency: code,
      notation: "compact",
      maximumFractionDigits: 1
    }).format(major);
  } catch {
    return formatMoney(amountMinor, currency);
  }
}
