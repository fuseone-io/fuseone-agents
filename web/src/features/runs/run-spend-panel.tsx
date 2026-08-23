import { Fragment } from "react";
import { useTranslation } from "react-i18next";
import { Panel } from "@/components/shared/panel";
import { formatBytes, formatCost, formatTokens } from "@/lib/format";
import { runSpend } from "@/features/runs/run-spend";
import type { UnpricedReason } from "@/features/runs/run-spend";
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
  // Named, because "tool results dominate" is a category and the next slice
  // needs a cause: which tool to compact.
  const tools = Object.entries(spend.byTool).sort((a, b) => b[1] - a[1]);
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
          {/* Zero has several causes; the ledger records which one this run hit. */}
          {!spend.priced && (
            <p className="mt-1 text-xs text-warning">
              {t(reasonKey(spend.reason))}
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
          {tools.length > 0 && (
            <>
              <h3 className="mt-4 text-2xs uppercase tracking-label text-muted-foreground">
                {t("runs.promptByTool")}
              </h3>
              <dl className="mt-2 grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-1 text-xs">
                {tools.map(([tool, value]) => (
                  <Fragment key={tool}>
                    <dt className="min-w-0 truncate font-mono">{tool}</dt>
                    <dd className="text-right font-mono tabular-nums text-muted-foreground">
                      {formatBytes(value)}
                    </dd>
                  </Fragment>
                ))}
              </dl>
            </>
          )}
          <p className="mt-3 text-2xs text-muted-foreground">
            {t("runs.promptCompositionUnit")}
          </p>

          {/* Two savings, never one number. Compaction kept bytes out of the
              prompt; the cache kept a call off somebody else's system and sent
              the same bytes anyway. A combined figure would send the reader to
              compact whatever the cache already covers. */}
          <h3 className="mt-4 text-2xs uppercase tracking-label text-muted-foreground">
            {t("runs.savedTitle")}
          </h3>
          {spend.elided === 0 && spend.cacheHits === 0 ? (
            <p className="mt-2 text-xs text-muted-foreground">
              {t("runs.savedNone")}
            </p>
          ) : (
            <ul className="mt-2 space-y-2">
              {spend.elided > 0 && (
                <li>
                  <div className="text-xs">
                    {t("runs.savedCompaction", { bytes: formatBytes(spend.elided) })}
                  </div>
                  <p className="text-2xs text-muted-foreground">
                    {t("runs.savedCompactionHelp")}
                  </p>
                </li>
              )}
              {spend.cacheHits > 0 && (
                <li>
                  <div className="text-xs">
                    {t("runs.savedCache", { count: spend.cacheHits })}
                  </div>
                  <p className="text-2xs text-muted-foreground">
                    {t("runs.savedCacheHelp")}
                  </p>
                </li>
              )}
            </ul>
          )}
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

function reasonKey(reason: UnpricedReason | undefined): string {
  switch (reason) {
    case "missing_rate":
      return "runs.spendMissingRate";
    case "partial_missing_rate":
      return "runs.spendPartialMissingRate";
    case "configured_zero":
      return "runs.spendConfiguredZero";
    case "rounded_zero":
      return "runs.spendRoundedZero";
    case "nothing_spent":
      return "runs.spendNothing";
    default:
      return "runs.spendUnknown";
  }
}
