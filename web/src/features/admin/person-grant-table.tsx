import type { ReactNode } from "react";
import { Check, GitBranch, Hand, Link } from "lucide-react";
import { useTranslation } from "react-i18next";
import {
  ROLE_ORDER,
  type Origin,
  type ScopeGrant,
} from "@/features/admin/person-access-model";
import { cn } from "@/lib/utils";

export function PersonGrantTable({ groups }: { groups: ScopeGrant[] }) {
  const { t } = useTranslation();
  return (
    <div className="overflow-x-auto">
      <table className="w-full min-w-[620px] text-sm">
        <thead className="bg-muted">
          <tr className="border-b border-border-subtle">
            <MatrixHead>{t("people.scope")}</MatrixHead>
            {ROLE_ORDER.map((role) => (
              <MatrixHead key={role} className="text-center">
                {t(`roles.${role}`)}
              </MatrixHead>
            ))}
            <MatrixHead>{t("people.origin")}</MatrixHead>
          </tr>
        </thead>
        <tbody>
          {groups.map((group) => (
            <tr
              key={group.scope}
              className="border-b border-border-subtle last:border-b-0"
            >
              <th
                scope="row"
                className="max-w-[180px] truncate px-3 py-2 text-left font-mono text-xs font-normal"
              >
                {group.scope}
              </th>
              {ROLE_ORDER.map((role) => (
                <td key={role} className="px-3 py-2 text-center">
                  {group.roles.includes(role) ? <Marked /> : <Unmarked />}
                </td>
              ))}
              <td className="px-3 py-2 text-xs text-muted-foreground">
                <OriginLabel origin={group.origin} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function MatrixHead({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <th
      scope="col"
      className={cn(
        "h-8 px-3 text-left text-2xs font-semibold uppercase tracking-label text-text-disabled",
        className,
      )}
    >
      {children}
    </th>
  );
}

function Marked() {
  return (
    <span className="inline-grid size-[18px] place-items-center rounded-[5px] bg-surface-accent text-text-accent">
      <Check className="size-3" aria-hidden />
    </span>
  );
}

function Unmarked() {
  return <span className="font-mono text-xs text-text-disabled">-</span>;
}

function OriginLabel({ origin }: { origin: Origin }) {
  const { t } = useTranslation();
  const Icon =
    origin === "provider" ? Link : origin === "mixed" ? GitBranch : Hand;
  return (
    <span className="inline-flex items-center gap-1.5">
      <Icon className="size-3 shrink-0" aria-hidden />
      {t(`people.origins.${origin}`)}
    </span>
  );
}
