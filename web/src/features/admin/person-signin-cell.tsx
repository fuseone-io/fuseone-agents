import type { TFunction } from "i18next";
import type { LucideIcon } from "lucide-react";
import { CircleSlash, Cloud, Lock } from "lucide-react";
import { useTranslation } from "react-i18next";
import { MobileColumnLabel } from "@/features/admin/person-access-summary";
import type { Person } from "@/features/admin/people-api";
import { formatRelative } from "@/lib/format";

export function PersonSignInCell({ person }: { person: Person }) {
  const { t } = useTranslation();
  const signIn = signInState(person, t);
  return (
    <div className="min-w-0">
      <MobileColumnLabel>{t("people.signIn")}</MobileColumnLabel>
      <span className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
        <signIn.Icon className="size-3.5 shrink-0" aria-hidden />
        <span className="min-w-0 truncate">{signIn.label}</span>
      </span>
    </div>
  );
}

export function PersonLastSeenCell({ lastSeen }: { lastSeen?: string }) {
  const { t } = useTranslation();
  return (
    <div className="min-w-0 text-xs text-muted-foreground">
      <MobileColumnLabel>{t("people.lastSeenColumn")}</MobileColumnLabel>
      <span className="whitespace-nowrap">
        {lastSeen ? formatRelative(lastSeen) : t("people.neverSeen")}
      </span>
    </div>
  );
}

function signInState(person: Person, t: TFunction): {
  Icon: LucideIcon;
  label: string;
} {
  if (person.provider) {
    return {
      Icon: Cloud,
      label: t("people.providerNamed", {
        provider: person.provider.replace(/^oidc:/, ""),
      }),
    };
  }
  if (person.kind !== "user") return { Icon: Lock, label: t("people.serviceToken") };
  if (person.username) return { Icon: Lock, label: t("people.localPassword") };
  return { Icon: CircleSlash, label: t("people.noPassword") };
}
