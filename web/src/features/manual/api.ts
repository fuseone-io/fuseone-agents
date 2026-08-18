import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

export type ManualEntry = components["schemas"]["ManualEntry"];
export type ManualPage = components["schemas"]["ManualPage"];

/** The two the manual is written in, which the contract names as an enum.
 *
 * The server falls back for anything else, and that is for callers other than
 * this one: the console knows both its languages, so sending it something the
 * contract does not allow would be relying on a fallback to cover a bug. */
type Locale = "pt-BR" | "en-US";

function useLocale(): Locale {
  const { i18n } = useTranslation();
  return i18n.language.startsWith("en") ? "en-US" : "pt-BR";
}

export const manualKeys = {
  all: ["manual"] as const,
  index: (locale: Locale) => [...manualKeys.all, "index", locale] as const,
  page: (locale: Locale, slug: string) =>
    [...manualKeys.all, "page", locale, slug] as const,
};

export function useManual() {
  const locale = useLocale();
  return useQuery({
    queryKey: manualKeys.index(locale),
    queryFn: async () =>
      unwrap(await api.GET("/manual", { params: { query: { locale } } })),
  });
}

export function useManualPage(slug: string) {
  const locale = useLocale();
  return useQuery({
    queryKey: manualKeys.page(locale, slug),
    queryFn: async () =>
      unwrap(
        await api.GET("/manual/{slug}", {
          params: { path: { slug }, query: { locale } },
        }),
      ),
  });
}
