/**
 * The console speaks two languages, and they are always in parity.
 *
 * The locale decides how text and numbers read. It does not decide the
 * currency: money crosses the wire as millionths of the *installation's*
 * currency, so switching to English must change "1.234,50" to "1,234.50" and
 * leave R$ as R$. A reader who switched language and saw dollars would be
 * looking at a different number.
 */
export const LOCALES = ["pt-BR", "en-US"] as const;

export type Locale = (typeof LOCALES)[number];

export const DEFAULT_LOCALE: Locale = "pt-BR";

export const LOCALE_NAMES: Record<Locale, string> = {
  "pt-BR": "Português (Brasil)",
  "en-US": "English (US)",
};

const KEY = "fuseone.locale";

export function isLocale(value: string | null | undefined): value is Locale {
  return LOCALES.includes(value as Locale);
}

/**
 * The stored choice, then the browser's, then Portuguese.
 *
 * Read through a guard because the console is installed inside the customer's
 * network and a locked-down browser profile can refuse storage outright.
 */
export function preferredLocale(): Locale {
  try {
    const stored = globalThis.localStorage?.getItem(KEY);
    if (isLocale(stored)) return stored;
  } catch {
    // Blocked. The choice lasts the session.
  }
  for (const tag of globalThis.navigator?.languages ?? []) {
    const match = LOCALES.find((l) =>
      l.toLowerCase().startsWith(tag.slice(0, 2).toLowerCase()),
    );
    if (match) return match;
  }
  return DEFAULT_LOCALE;
}

export function rememberLocale(locale: Locale): void {
  try {
    globalThis.localStorage?.setItem(KEY, locale);
  } catch {
    // Blocked; nothing to do but carry on.
  }
}
