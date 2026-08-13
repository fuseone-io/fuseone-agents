import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Panel } from "@/components/shared/panel";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import { EraseDialog } from "@/features/admin/erase-dialog";
import { useRetention, useSetRetention } from "@/features/admin/retention-api";

/**
 * How long content is kept, and the one way to remove it early.
 *
 * Content, never the ledger: the trail is immutable and survives every
 * erasure, because what goes is the referenced payload and the step keeps its
 * reference and its digest. The screen says so, because "delete my data" and
 * "delete the record that anything happened" are different requests and only
 * one of them is possible here.
 */
export function RetentionPanel() {
  const { t } = useTranslation();
  const { data, isLoading, error, refetch } = useRetention();
  const save = useSetRetention();
  const [days, setDays] = useState<string>("");
  const [erasing, setErasing] = useState(false);

  const current = data?.days ?? 0;
  const submit = () =>
    save.mutate(Number(days), {
      onSuccess: () => {
        toast.success(t("retention.saved"), {
          description: t("retention.savedHint"),
        });
        setDays("");
      },
      onError: (e) =>
        toast.error(t("retention.saveFailed"), {
          description: e instanceof Error ? e.message : undefined,
        }),
    });

  return (
    <Panel
      title={t("retention.title")}
      action={
        <Button variant="outline" size="sm" onClick={() => setErasing(true)}>
          {t("retention.eraseAction")}
        </Button>
      }
    >
      {isLoading ? (
        <LoadingRows rows={2} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : (
        <div className="flex max-w-2xl flex-col gap-4">
          <p className="text-sm text-muted-foreground">
            {t("retention.explains")}
          </p>

          <div className="flex items-center gap-2">
            <span className="text-sm">
              {t("retention.current", { days: current })}
            </span>
            {data?.configured === false && (
              <Badge variant="outline">{t("retention.byDefault")}</Badge>
            )}
          </div>

          <div className="flex items-end gap-2">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="retention-days">{t("retention.days")}</Label>
              <Input
                id="retention-days"
                type="number"
                min={1}
                value={days}
                onChange={(e) => setDays(e.target.value)}
                className="w-32 font-mono"
                placeholder={String(current)}
              />
            </div>
            <Button
              onClick={submit}
              disabled={days === "" || Number(days) < 1 || save.isPending}
            >
              {t("common.save")}
            </Button>
          </div>

          {/* Said before it is done, not after: shortening the window erases
              on the next sweep, and nobody can put it back. */}
          <p className="text-xs text-muted-foreground">
            {t("retention.shorteningWarns")}
          </p>
        </div>
      )}

      {erasing && <EraseDialog onClose={() => setErasing(false)} />}
    </Panel>
  );
}
