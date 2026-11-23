import { useState } from "react";
import { Globe, Plus } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { ConfirmRemove } from "@/features/integrations/egress/confirm-remove";
import { EgressPolicyDialog } from "@/features/integrations/egress/egress-policy-dialog";
import { scopeLabel, useDeleteEgressPolicy, useEgressPolicies, type EgressPolicy } from "@/features/integrations/egress/api";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * Where MCP servers outside the installation may send requests (NT-019).
 * One list of exact origins per company or area; an area's servers are held
 * to the area's list, or else the company's, and with neither they reach
 * nothing. The installation's servers are held to none.
 */
export function EgressPoliciesPanel() {
  const { t } = useTranslation();
  const { data, isLoading, error, refetch } = useEgressPolicies();
  const remove = useDeleteEgressPolicy();
  const [editing, setEditing] = useState<EgressPolicy | null | undefined>(undefined);
  const [removing, setRemoving] = useState<EgressPolicy | null>(null);
  const policies = data?.policies ?? [];

  if (isLoading) return <LoadingRows rows={3} />;
  if (error) return <ErrorState error={error} onRetry={() => void refetch()} />;

  return (
    <section className="flex flex-col gap-3">
      <div className="flex items-start justify-between gap-3">
        <p className="text-sm text-muted-foreground">{t("integrations.egress.intro")}</p>
        <Button onClick={() => setEditing(null)}>
          <Plus className="size-4" aria-hidden />
          {t("integrations.egress.new")}
        </Button>
      </div>
      {policies.length === 0 ? (
        <EmptyState icon={<Globe className="size-6" />} title={t("integrations.egress.none")} hint={t("integrations.egress.noneHint")} />
      ) : (
        <ul className="divide-y rounded-xl border bg-card shadow-sm">
          {policies.map((policy) => (
            <li key={scopeLabel(policy)} className="flex items-start gap-3 p-4">
              <div className="min-w-0 flex-1 space-y-1">
                <Mono className="text-sm font-medium">{scopeLabel(policy)}</Mono>
                {policy.origins.length === 0 ? (
                  <p className="text-xs text-muted-foreground">{t("integrations.egress.reachesNothing")}</p>
                ) : (
                  policy.origins.map((origin) => <Mono key={origin} className="block text-xs">{origin}</Mono>)
                )}
              </div>
              <Button variant="ghost" size="sm" onClick={() => setEditing(policy)}>{t("agents.edit")}</Button>
              <Button variant="ghost" size="sm" onClick={() => setRemoving(policy)}>{t("common.remove")}</Button>
            </li>
          ))}
        </ul>
      )}
      {editing !== undefined && (
        <EgressPolicyDialog key={editing ? scopeLabel(editing) : "new"} policy={editing} policies={policies} onClose={() => setEditing(undefined)} />
      )}
      <ConfirmRemove
        policy={removing}
        onCancel={() => setRemoving(null)}
        onConfirm={(policy) =>
          remove.mutate(policy, {
            onSuccess: () => setRemoving(null),
            onError: (problem) => toast.error(problemMessage(problem, t)),
          })
        }
      />
    </section>
  );
}
