import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import type { ServiceToken } from "@/features/admin/service-accounts-api";
import { tokenState } from "@/features/admin/service-token-state";
import { formatInstant } from "@/lib/format";

const BADGE = {
  active: "secondary",
  expired: "outline",
  revoked: "destructive",
} as const;

/**
 * Names repeat, because rotating keeps one, so the id is what tells two rows
 * apart. A revoked token has nothing left to do to it; an expired one cannot
 * authenticate, so it needs a replacement rather than a revocation.
 */
export function ServiceTokenRow({
  token,
  mayIssue,
  onRotate,
  onRevoke,
}: {
  token: ServiceToken;
  mayIssue: boolean;
  onRotate: () => void;
  onRevoke: () => void;
}) {
  const { t } = useTranslation();
  const state = tokenState(token);
  return (
    <div className="flex flex-wrap items-center gap-3 py-2.5">
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm font-medium">{token.name}</span>
          <Mono dim>{token.id}</Mono>
          <Badge variant={BADGE[state]}>{t(`serviceAccounts.token.${state}`)}</Badge>
        </div>
        <p className="text-xs text-muted-foreground">
          {[
            t("serviceAccounts.issuedBy", {
              who: token.createdBy,
              when: formatInstant(token.createdAt),
            }),
            token.revokedAt
              ? t("serviceAccounts.revokedAt", { when: formatInstant(token.revokedAt) })
              : t("serviceAccounts.expiresAt", { when: formatInstant(token.expiresAt) }),
            token.lastUsedAt
              ? t("serviceAccounts.lastUsed", { when: formatInstant(token.lastUsedAt) })
              : t("serviceAccounts.neverUsed"),
          ].join(" · ")}
        </p>
      </div>
      <div className="flex gap-2">
        {state !== "revoked" && mayIssue && (
          <Button
            variant="outline"
            size="sm"
            aria-label={t("serviceAccounts.rotateNamed", { name: token.name })}
            onClick={onRotate}
          >
            {t("serviceAccounts.rotate")}
          </Button>
        )}
        {state === "active" && (
          <Button
            variant="outline"
            size="sm"
            aria-label={t("serviceAccounts.revokeNamed", { name: token.name })}
            onClick={onRevoke}
          >
            {t("serviceAccounts.revoke")}
          </Button>
        )}
      </div>
    </div>
  );
}
