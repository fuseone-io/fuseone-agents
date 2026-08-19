import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import type { Person } from "@/features/admin/people-api";

export function PersonIdentityCell({ person }: { person: Person }) {
  const { t } = useTranslation();
  return (
    <div className="flex min-w-0 items-center gap-2.5">
      <span className="grid size-[30px] shrink-0 place-items-center rounded-full border border-border bg-surface-accent text-xs font-medium text-text-accent">
        {initials(person.display || person.email || person.id)}
      </span>
      <div className="min-w-0">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="min-w-0 truncate font-medium">{person.display}</span>
          {person.kind !== "user" && (
            <span className="rounded-full border px-2 py-0.5 text-xs text-muted-foreground">
              {t(`people.kind.${person.kind}`)}
            </span>
          )}
          {person.disabled && (
            <span className="rounded-full bg-destructive px-2 py-0.5 text-xs font-medium text-white">
              {t("people.disabled")}
            </span>
          )}
        </div>
        <Mono dim className="block truncate text-[11px]">
          {person.email || person.id}
        </Mono>
      </div>
    </div>
  );
}

function initials(value: string) {
  const letters = value
    .trim()
    .split(/\s+/)
    .slice(0, 2)
    .map((part) => part[0])
    .join("");
  return (letters || "?").toUpperCase();
}
