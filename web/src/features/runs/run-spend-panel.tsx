import { useTranslation } from "react-i18next";
import { Panel } from "@/components/shared/panel";
import { formatBytes, formatCost, formatTokens } from "@/lib/format";
import { runSpend } from "@/features/runs/run-spend";
import type { Run, Step } from "@/lib/api/client";

/**
 * Where a run's spend came from.
 *
 * Two measurements side by side, never combined. Tokens are what the provider
 * reported and the only thing money is derived from; bytes are what this
 * platform measured while assembling the prompt, and they answer a different
 * question — which part of the input is large enough to be worth compacting.
 * Dividing one by the other would produce a rate nobody set.
 *
 * The content itself is deliberately absent. Explaining a cost invites showing
 * the prompt, and the prompt lives in the content store under retention and
 * erasure behind another permission. Sizes are safe here; the text is not.
 */
export function RunSpendPanel({ run, steps }: { run: Run; steps: Step[] }) {
  const { t } = useTranslation();
  const spend = runSpend(run.cost, steps);
  const sources = Object.entries(spend.bytes).sort((a, b) => b[1] - a[1]);
  const total = sources.reduce((sum, [, value]) => sum + value, 0);

  return (
    <Panel title={t("runs.spendTitle")}>
      <p className="mb-4 max-w-[70ch] text-xs text-muted-foreground">
        {t("runs.spendHelp")}
      </p>
      <div className="grid gap-4 md:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)]">
        <div className="min-w-0">
          <div className="font-mono text-[22px]/7 font-medium tabular-nums">
            {formatCost(run.cost)}
          </div>
          {/* Zero has two causes and only one of them is a price. */}
          {!spend.priced && (
            <p className="mt-1 text-xs text-warning">
              {spend.reason === "no_rate"
                ? t("runs.spendNoRate")
                : t("runs.spendNothing")}
            </p>
          )}
          <dl className="mt-3 grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
            <Row label={t("runs.tokensIn")} value={run.cost.inputTokens} />
            <Row label={t("runs.tokensOut")} value={run.cost.outputTokens} />
            <Row label={t("runs.tokensCacheRead")} value={run.cost.cacheReadTokens} />
            <Row label={t("runs.tokensCacheWrite")} value={run.cost.cacheWriteTokens} />
          </dl>
        </div>

        <div className="min-w-0">
          <h3 className="text-2xs uppercase tracking-label text-muted-foreground">
            {t("runs.promptComposition")}
          </h3>
          {sources.length === 0 ? (
            <p className="mt-2 text-xs text-muted-foreground">
              {t("runs.promptCompositionEmpty")}
            </p>
          ) : (
            <ul className="mt-2 space-y-1.5">
              {sources.map(([source, value]) => (
                <li key={source} className="min-w-0">
                  <div className="flex items-baseline justify-between gap-3 text-xs">
                    <span className="min-w-0 break-words">
                      {t(`runs.promptSource.${source}`, { defaultValue: source })}
                    </span>
                    <span className="font-mono tabular-nums text-muted-foreground">
                      {formatBytes(value)}
                    </span>
                  </div>
                  <div className="mt-1 h-1 rounded-full bg-muted">
                    <div
                      className="h-1 rounded-full bg-primary"
                      style={{ width: `${total > 0 ? (value / total) * 100 : 0}%` }}
                    />
                  </div>
                </li>
              ))}
            </ul>
          )}
          <p className="mt-2 text-2xs text-muted-foreground">
            {t("runs.promptCompositionUnit")}
          </p>
        </div>
      </div>
    </Panel>
  );
}

function Row({ label, value }: { label: string; value?: number }) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="text-right font-mono tabular-nums">
        {formatTokens(value ?? 0)}
      </dd>
    </>
  );
}
