import { AlertTriangle, Copy } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { failureLabel } from "@/features/runtime/failure-labels";
import type { Run } from "@/lib/api/client";

export function RunFailureNotice({ run }: { run: Run }) {
  const { t } = useTranslation();
  const failure = run.failure;
  if (!failure) return null;

  const copy = async () => {
    if (failure.requestId) await navigator.clipboard.writeText(failure.requestId);
  };

  return (
    <section className="rounded-xl border border-warning bg-warning-surface p-4 text-warning">
      <div className="flex min-w-0 items-start gap-3">
        <AlertTriangle className="mt-0.5 size-5 shrink-0" />
        <div className="min-w-0 flex-1">
          <h2 className="font-medium">{t(failureLabel(failure.code))}</h2>
          <p className="mt-1 text-sm text-warning">
            {t(runFailureDetail(failure.code), {
              provider: failure.provider || t("runtime.providerUnknown"),
              status: failure.status || t("runtime.noStatus"),
            })}
          </p>
          {failure.requestId && (
            <div className="mt-3 flex min-w-0 flex-wrap items-center gap-2 text-xs">
              <span className="text-warning">{t("runtime.requestId")}</span>
              <code className="min-w-0 break-all rounded bg-background px-2 py-1 text-foreground">
                {failure.requestId}
              </code>
              <Button variant="ghost" size="icon" className="size-7" onClick={() => void copy()}>
                <Copy className="size-3.5" />
                <span className="sr-only">{t("runtime.copyRequestId")}</span>
              </Button>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}

function runFailureDetail(code: string): string {
  if (code === "dedupe_in_flight") return "runtime.runFailureDedupeInFlightDetail";
  return "runtime.runFailureDetail";
}
