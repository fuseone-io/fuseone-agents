import { Trans, useTranslation } from "react-i18next";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { ShieldCheck } from "lucide-react";
import { Mono } from "@/components/shared/mono";
import { toolsOf } from "@/features/runs/run-tools";
import { formatTime, shortHash } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { Run, Step } from "@/lib/api/client";

/**
 * The three questions somebody asks about a run without reading it: whose it
 * is, what it touched, and whether the record can be trusted.
 */
export function RunSideRail({ run, steps }: { run: Run; steps: Step[] }) {
  const { t } = useTranslation();
  const tools = toolsOf(steps);
  const wrote = tools.some((tool) => tool.wrote);
  const last = steps[steps.length - 1];

  return (
    <aside className="flex flex-col gap-3 lg:sticky lg:top-0">
      <Card title="Resumo">
        <Row label="Agente">
          <Link
            to={`/agents/${run.agentId}`}
            className="text-sm text-primary hover:underline"
          >
            {run.agentId}
          </Link>
        </Row>
        <Row label={t("runs.version")}>
          <Mono>{run.versionId.slice(0, 9)}</Mono>
        </Row>
        <Row label="Área">
          <span className="text-sm">{run.scope.area || "—"}</span>
        </Row>
        <Row label="Em nome de">
          <span className="text-sm">{run.onBehalfOf ?? "—"}</span>
        </Row>
      </Card>

      <Card title="Ferramentas usadas">
        {tools.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            {t("runs.noToolsCalled")}
          </p>
        ) : (
          <div className="flex flex-wrap gap-1.5">
            {tools.map((tool) => (
              <span
                key={tool.name}
                className={cn(
                  "rounded-md border px-1.5 py-0.5 font-mono text-2xs",
                  tool.escalated
                    ? "border-warning bg-warning-surface text-warning"
                    : "border-border",
                )}
              >
                {tool.name}
              </span>
            ))}
          </div>
        )}
        {/* Whether anything actually changed is the first thing asked after an
            incident, and it is not answerable by reading a list of names. */}
        <p className="text-xs text-muted-foreground">
          {wrote ? t("runs.changedState") : t("runs.noWrites")}
        </p>
      </Card>

      <Card title="Integridade">
        <p className="flex items-center gap-2 text-sm">
          <ShieldCheck className="size-4 text-success" aria-hidden />
          {t("runs.chainSealed", { count: steps.length })}
        </p>
        {last && (
          <p className="text-xs text-muted-foreground">
            <Trans
              i18nKey="runs.lastSealLine"
              values={{ hash: shortHash(last.hash), at: formatTime(last.at) }}
              components={{ h: <Mono dim /> }}
            />
          </p>
        )}
        <p className="text-xs text-muted-foreground">
          {t("runs.verifyExplains")}
        </p>
      </Card>
    </aside>
  );
}

function Card({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-2.5 rounded-xl border border-border bg-card p-4 shadow-sm">
      <h2 className="text-2xs uppercase tracking-label text-muted-foreground">
        {title}
      </h2>
      {children}
    </section>
  );
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-3">
      <span className="text-xs text-muted-foreground">{label}</span>
      {children}
    </div>
  );
}
