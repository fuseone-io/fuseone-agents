import i18next from "i18next";
import { initReactI18next } from "react-i18next";
import ptBR from "@/i18n/pt-BR.json";
import enUS from "@/i18n/en-US.json";
import { DEFAULT_LOCALE, preferredLocale, rememberLocale, type Locale } from "@/i18n/locale";

/**
 * Both catalogues ship in the bundle rather than being fetched.
 *
 * The console runs inside the customer's network and is served by the Go
 * binary through go:embed. A locale loaded over the network would be one more
 * request that can fail, and its failure mode is a screen of key names.
 */
void i18next.use(initReactI18next).init({
  resources: { "pt-BR": { translation: ptBR }, "en-US": { translation: enUS } },
  lng: preferredLocale(),
  fallbackLng: DEFAULT_LOCALE,
  interpolation: { escapeValue: false },
  // A missing key renders its own name, loudly, rather than silently falling
  // back to a language the reader may not have.
  returnEmptyString: false,
});

export function setLocale(locale: Locale): void {
  rememberLocale(locale);
  void i18next.changeLanguage(locale);
}

export function currentLocale(): Locale {
  return (i18next.resolvedLanguage ?? DEFAULT_LOCALE) as Locale;
}

export { i18next };
