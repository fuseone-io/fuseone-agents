import { Trans, useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { SimulationCard } from "@/features/policies/simulation-card";
import { draftSentence } from "@/features/policies/policy-sentence";
import type { Change } from "@/features/policies/policy-form";
import type { PolicyInput } from "@/lib/api/client";

/**
 * What saving this will mean.
 *
 * The handoff puts a simulation here — the draft run against the last 500
 * runs, with the count of would-be denials. That needs replay, which does not
 * exist yet, so this rail says what it can say honestly and states the gap
 * rather than showing a reassuring number nobody computed.
 */
export function PolicySideRail({
  draft,
  creating,
  changes,
}: {
  draft: PolicyInput;
  creating: boolean;
  changes: Change[];
}) {
  const { t } = useTranslation();
  return (
    <aside className="flex flex-col gap-3 lg:sticky lg:top-0">
      <Card title={t("policies.theRule")}>
        <Mono className="block break-words text-xs">
          {draftSentence(draft)}
        </Mono>
        <p className="text-xs text-muted-foreground">
          {draft.mode === "monitor"
            ? t("policies.willBeMonitored")
            : t("policies.willApplyAfterReload")}
        </p>
      </Card>

      {creating ? (
        <Card title={t("policies.evaluationOrder")}>
          <p className="text-xs text-muted-foreground">
            {t("policies.mostRestrictiveWins")}
          </p>
          <p className="text-xs text-muted-foreground">
            <Trans
              i18nKey="policies.onlyExceptionIsAllow"
              components={{ em: <span className="text-foreground" /> }}
            />
          </p>
        </Card>
      ) : (
        <Card title={`Sem gravar (${changes.length})`}>
          {changes.length === 0 ? (
            <p className="text-xs text-muted-foreground">
              {t("policies.nothingChangedYet")}
            </p>
          ) : (
            <ul className="flex flex-col gap-1.5">
              {changes.map((change) => (
                <li key={t(change.field)} className="text-xs">
                  <span className="text-warning">~</span> {t(change.field)}{" "}
                  <Mono dim className="text-2xs">
                    {change.from} → {change.to}
                  </Mono>
                </li>
              ))}
            </ul>
          )}
        </Card>
      )}

      <SimulationCard draft={draft} />
    </aside>
  );
}

function Card({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <section className="flex flex-col gap-2 rounded-xl border border-border bg-card p-4 shadow-sm">
      <h2 className="text-2xs uppercase tracking-label text-muted-foreground">
        {title}
      </h2>
      {children}
    </section>
  );
}
