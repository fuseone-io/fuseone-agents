import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Plus, ShieldCheck } from "lucide-react";
import { toast } from "sonner";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import { Panel } from "@/components/shared/panel";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { StandingForm } from "@/features/admin/standing-form";
import {
  useRevokeStandingApproval,
  useStandingApprovals,
  type StandingApproval,
} from "@/features/admin/standing-api";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * The durable human grants: who pre-approved which tool for which agent,
 * how much of today's ceiling is spent, and when the mandate ends. Every
 * release a grant produces carries its owner's name in the run's own trail.
 */
export function StandingPanel() {
  const { t } = useTranslation();
  const { data, isLoading, error, refetch } = useStandingApprovals();
  const [creating, setCreating] = useState(false);
  const [revoking, setRevoking] = useState<StandingApproval | null>(null);
  const revoke = useRevokeStandingApproval();
  const grants = data ?? [];

  return (
    <Panel
      title={t("admin.standing")}
      action={
        <Button size="sm" onClick={() => setCreating(true)}>
          <Plus className="size-4" aria-hidden />
          {t("admin.standingNew")}
        </Button>
      }
    >
      {isLoading ? (
        <LoadingRows rows={3} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : grants.length === 0 ? (
        <EmptyState
          icon={<ShieldCheck className="size-5" aria-hidden />}
          title={t("admin.standingEmpty")}
          hint={t("admin.standingEmptyHint")}
        />
      ) : (
        <ul className="flex flex-col gap-2">
          {grants.map((grant) => (
            <li
              key={grant.id}
              className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 rounded-lg border p-3 text-sm"
            >
              <Mono className="shrink-0">{grant.toolId}</Mono>
              <span className="text-muted-foreground">→</span>
              <Mono>{grant.agentId}</Mono>
              <Badge
                variant={grant.status === "active" ? "default" : "outline"}
              >
                {t(
                  grant.status === "active"
                    ? "admin.standingActive"
                    : "admin.standingRevoked",
                )}
              </Badge>
              <span className="text-xs text-muted-foreground">
                {t("admin.standingUses", {
                  used: grant.usesToday,
                  cap: grant.dailyCap,
                })}
                {" · "}
                {t("admin.standingBy", { who: grant.createdBy })}
                {" · "}
                {t("admin.standingUntil", {
                  when: new Date(grant.expiresAt).toLocaleString(),
                })}
              </span>
              <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
                {grant.reason}
              </span>
              {grant.status === "active" && (
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => setRevoking(grant)}
                >
                  {t("admin.standingRevoke")}
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}

      {creating && <StandingForm onClose={() => setCreating(false)} />}

      <AlertDialog
        open={revoking !== null}
        onOpenChange={(open) => !open && setRevoking(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("admin.standingRevokeTitle")}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t("admin.standingRevokeBody", {
                tool: revoking?.toolId,
                agent: revoking?.agentId,
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (!revoking) return;
                revoke.mutate(revoking.id, {
                  onSuccess: () => toast.success(t("admin.standingRevoked")),
                  onError: (err) => toast.error(problemMessage(err, t)),
                });
                setRevoking(null);
              }}
            >
              {t("admin.standingRevoke")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Panel>
  );
}
