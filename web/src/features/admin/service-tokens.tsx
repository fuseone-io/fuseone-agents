import { useState } from "react";
import { useTranslation } from "react-i18next";
import { KeyRound, Plus, TriangleAlert } from "lucide-react";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { Button } from "@/components/ui/button";
import { ServiceAccountSection } from "@/features/admin/service-account-section";
import {
  useServiceTokens,
  type ServiceAccount,
  type ServiceToken,
} from "@/features/admin/service-accounts-api";
import { ServiceTokenForm } from "@/features/admin/service-token-form";
import { ServiceTokenRevoke } from "@/features/admin/service-token-revoke";
import { ServiceTokenRow } from "@/features/admin/service-token-row";
import { ServiceTokenSecret } from "@/features/admin/service-token-secret";

type Issuing = { replacing?: ServiceToken };
type Revealed = { secret: string; replacing?: ServiceToken };

/**
 * The secret lives in `revealed`, this state and nowhere else, and leaves it
 * when the dialog closes.
 */
export function ServiceTokens({ account }: { account: ServiceAccount }) {
  const { t } = useTranslation();
  const { data, isLoading, error, refetch } = useServiceTokens(account.id);
  const [issuing, setIssuing] = useState<Issuing | null>(null);
  const [revoking, setRevoking] = useState<ServiceToken | null>(null);
  const [revealed, setRevealed] = useState<Revealed | null>(null);
  const tokens = data?.items ?? [];
  const mayIssue = !account.disabled;

  return (
    <ServiceAccountSection
      title={t("serviceAccounts.tokensTitle")}
      explains={t("serviceAccounts.tokensExplains")}
      action={
        mayIssue && (
          <Button variant="outline" size="sm" onClick={() => setIssuing({})}>
            <Plus className="size-4" aria-hidden />
            {t("serviceAccounts.issue")}
          </Button>
        )
      }
    >
      {!mayIssue && (
        <p className="flex items-start gap-2 text-xs text-muted-foreground">
          <TriangleAlert
            className="mt-px size-3.5 shrink-0 text-warning"
            aria-hidden
          />
          <span>{t("serviceAccounts.disabledNoIssue")}</span>
        </p>
      )}
      {isLoading ? (
        <LoadingRows rows={2} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : tokens.length === 0 ? (
        <EmptyState
          icon={<KeyRound className="size-6" />}
          title={t("serviceAccounts.noTokensTitle")}
          hint={t("serviceAccounts.noTokensHint")}
        />
      ) : (
        <ul className="divide-y divide-border-subtle">
          {tokens.map((token) => (
            <li key={token.id}>
              <ServiceTokenRow
                token={token}
                mayIssue={mayIssue}
                onRotate={() => setIssuing({ replacing: token })}
                onRevoke={() => setRevoking(token)}
              />
            </li>
          ))}
        </ul>
      )}

      {issuing && (
        <ServiceTokenForm
          account={account.id}
          replacing={issuing.replacing}
          onClose={() => setIssuing(null)}
          onIssued={(secret) => {
            setRevealed({ secret, replacing: issuing.replacing });
            setIssuing(null);
          }}
        />
      )}
      {revoking && (
        <ServiceTokenRevoke
          account={account.id}
          token={revoking}
          onClose={() => setRevoking(null)}
        />
      )}
      {revealed && (
        <ServiceTokenSecret
          secret={revealed.secret}
          replacing={revealed.replacing}
          onClose={() => setRevealed(null)}
        />
      )}
    </ServiceAccountSection>
  );
}
