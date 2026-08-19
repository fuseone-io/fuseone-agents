import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import { GrantBadge } from "@/features/admin/grant-badge";
import { formatRelative } from "@/lib/format";
import type { Person } from "@/features/admin/people-api";

/**
 * One person, and everything they hold.
 *
 * Holding nothing is stated rather than left blank: somebody who can sign in
 * and do nothing looks identical to somebody nobody has got to yet, and they
 * are the same problem.
 */
export function PersonRow({
  person,
  onEdit,
  onSetPassword,
}: {
  person: Person;
  onEdit: () => void;
  onSetPassword: () => void;
}) {
  const { t } = useTranslation();
  const grants = person.grants ?? [];

  return (
    <div className="grid gap-3 rounded-lg border p-3 lg:grid-cols-[minmax(0,1.1fr)_minmax(0,1.5fr)_minmax(128px,auto)_minmax(136px,auto)] lg:items-center">
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="min-w-0 break-words font-medium">
            {person.display}
          </span>
          {person.kind !== "user" && (
            <Badge variant="outline">{t(`people.kind.${person.kind}`)}</Badge>
          )}
          {person.disabled && (
            <Badge variant="destructive">{t("people.disabled")}</Badge>
          )}
        </div>
        <Mono dim className="block truncate text-xs">
          {person.email || person.id}
        </Mono>
      </div>

      <div className="min-w-0">
        <p className="mb-1 text-2xs font-medium uppercase tracking-normal text-muted-foreground lg:hidden">
          {t("people.access")}
        </p>
        <div className="flex min-w-0 flex-wrap items-center gap-1.5">
          {grants.length === 0 ? (
            <Badge variant="destructive">{t("people.noAccess")}</Badge>
          ) : (
            grants.map((grant, i) => <GrantBadge key={i} grant={grant} />)
          )}
        </div>
      </div>

      <div className="min-w-0 text-xs text-muted-foreground">
        <p className="mb-1 text-2xs font-medium uppercase tracking-normal text-muted-foreground lg:hidden">
          {t("people.lastActivity")}
        </p>
        {person.lastSeen
          ? t("people.lastSeen", { when: formatRelative(person.lastSeen) })
          : t("people.neverSeen")}
      </div>

      {/* Only where a password is the way in. Somebody a provider vouched
          for signs in there, and offering to set one here would invite two
          credentials for one person. */}
      <div className="flex min-w-0 flex-wrap items-center justify-start gap-2 lg:justify-end">
        {person.kind === "user" &&
          !(person.provider ?? "").startsWith("oidc") && (
            <Button
              variant="ghost"
              size="sm"
              className="h-7"
              onClick={onSetPassword}
            >
              {person.username
                ? t("people.changePassword")
                : t("people.setPassword")}
            </Button>
          )}

        <Button variant="ghost" size="sm" className="h-7" onClick={onEdit}>
          {t("people.manage")}
        </Button>
      </div>
    </div>
  );
}
