import { Link } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { roleSummary, type ScopeGrant } from "@/features/admin/person-access-model";

export function PersonAccessChip({ group }: { group: ScopeGrant }) {
  const { t } = useTranslation();
  const title = t("people.scopeGrantTitle", {
    scope: group.scope,
    roles: group.roles.map((role) => t(`roles.${role}`)).join(", "),
    origin: t(`people.origins.${group.origin}`),
  });

  return (
    <span
      title={title}
      className="inline-flex h-6 max-w-full items-center gap-1.5 rounded-md border border-border bg-muted px-2"
    >
      {group.origin !== "local" && (
        <Link className="size-3 shrink-0 text-muted-foreground" aria-hidden />
      )}
      <Mono className="truncate text-[11px]">{group.scope}</Mono>
      <span className="shrink-0 text-[11px] text-muted-foreground">
        {roleSummary(group.roles, t)}
      </span>
    </span>
  );
}
