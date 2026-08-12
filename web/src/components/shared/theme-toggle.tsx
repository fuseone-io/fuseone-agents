import { useTranslation } from "react-i18next";
import { useTheme } from "next-themes";
import { Monitor, Moon, Sun } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

const OPTIONS = [
  { value: "light", label: "theme.light", icon: Sun },
  { value: "dark", label: "theme.dark", icon: Moon },
  { value: "system", label: "theme.system", icon: Monitor },
] as const;

/**
 * A plain toggle would drop "follow the system", which is the setting most
 * people have already made once and should not have to make again per app.
 */
export function ThemeToggle() {
  const { t } = useTranslation();
  const { theme, setTheme } = useTheme();

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" aria-label={t("theme.change")}>
          <Sun className="size-4 dark:hidden" />
          <Moon className="hidden size-4 dark:block" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {OPTIONS.map(({ value, label, icon: Icon }) => (
          <DropdownMenuItem
            key={value}
            onSelect={() => setTheme(value)}
            data-active={theme === value}
            className="data-[active=true]:text-text-accent"
          >
            <Icon className="size-4" />
            {t(label)}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
