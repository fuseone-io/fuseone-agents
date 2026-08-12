import { useTranslation } from "react-i18next";
import { Languages } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { setLocale } from "@/i18n";
import { LOCALES, LOCALE_NAMES, isLocale } from "@/i18n/locale";

/**
 * Beside the theme, because both are the same kind of choice: how this person
 * wants the console to read, remembered for them and nobody else.
 *
 * Each language names itself in its own words. "Portuguese" is no use to
 * somebody who cannot read the English it is written in.
 */
export function LanguageToggle() {
  const { t, i18n } = useTranslation();

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" aria-label={t("language.change")}>
          <Languages className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {LOCALES.map((locale) => (
          <DropdownMenuItem
            key={locale}
            onSelect={() => setLocale(locale)}
            data-active={isLocale(i18n.resolvedLanguage) && i18n.resolvedLanguage === locale}
            className="data-[active=true]:text-text-accent"
          >
            {LOCALE_NAMES[locale]}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
