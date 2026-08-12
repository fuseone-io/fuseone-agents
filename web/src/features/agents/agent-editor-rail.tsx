import { Trans, useTranslation } from "react-i18next";
import { CircleCheck, CircleDashed, ShieldAlert } from "lucide-react";
import { Mono } from "@/components/shared/mono";
import { riskSurface } from "@/features/agents/risk-surface";
import type { Change } from "@/features/agents/agent-draft";
import type { AgentDefinition, Tool } from "@/lib/api/client";

/**
 * What publishing this will mean.
 *
 * Creating shows what is still missing; editing shows what moved. Both end
 * with what the agent will be able to touch, because that is the question
 * whoever approves this is actually answering.
 */
export function AgentEditorRail({
  draft,
  catalogue,
  creating,
  changes,
}: {
  draft: AgentDefinition;
  catalogue: Tool[];
  creating: boolean;
  changes: Change[];
}) {
  const { t } = useTranslation();
  return (
    <aside className="flex flex-col gap-3 lg:sticky lg:top-0">
      {creating ? <Checklist draft={draft} /> : <Diff changes={changes} />}

      <Card title={t("agents.riskSurface")}>
        <ul className="flex flex-col gap-1">
          {riskSurface(draft.tools ?? [], catalogue).map((line) => (
            <li
              key={line}
              className="flex items-start gap-2 text-xs text-muted-foreground"
            >
              <ShieldAlert className="mt-px size-3.5 shrink-0" aria-hidden />
              {line}
            </li>
          ))}
        </ul>
      </Card>
    </aside>
  );
}

/** What is still missing, and what happens when it is not. */
function Checklist({ draft }: { draft: AgentDefinition }) {
  const { t } = useTranslation();
  const items = [
    {
      done: draft.name !== "" && draft.area !== "",
      label: t("agents.nameAndArea"),
    },
    {
      done: draft.provider !== "" && draft.model !== "",
      label: "Provedor e modelo",
    },
    { done: draft.instructions.trim() !== "", label: t("agents.instructions") },
    { done: (draft.tools ?? []).length > 0, label: "Ao menos uma ferramenta" },
    {
      done: (draft.budget?.micros ?? 0) > 0 || (draft.budget?.steps ?? 0) > 0,
      label: "Um teto de custo ou de passos",
    },
  ];

  return (
    <Card title="Antes de publicar">
      <ul className="flex flex-col gap-1.5">
        {items.map((item) => (
          <li key={item.label} className="flex items-center gap-2 text-xs">
            {item.done ? (
              <CircleCheck
                className="size-3.5 shrink-0 text-success"
                aria-hidden
              />
            ) : (
              <CircleDashed
                className="size-3.5 shrink-0 text-text-disabled"
                aria-hidden
              />
            )}
            <span className={item.done ? "" : "text-muted-foreground"}>
              {item.label}
            </span>
          </li>
        ))}
      </ul>
      <p className="text-xs text-muted-foreground">
        {/* One key, not three. A sentence split around its emphasis cannot be
            translated: word order differs between languages, and three
            fragments give a translator no way to reorder them. */}
        <Trans
          i18nKey="agents.pausedOnCreate"
          components={{ em: <span className="text-foreground" /> }}
        />
      </p>
    </Card>
  );
}

/** What moved since the version that is published. */
function Diff({ changes }: { changes: Change[] }) {
  const { t } = useTranslation();
  return (
    <Card title={`Sem publicar (${changes.length})`}>
      {changes.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t("agents.nothingChanged")}
        </p>
      ) : (
        <ul className="flex flex-col gap-1.5">
          {changes.map((change) => (
            <li key={change.field} className="text-xs">
              <span className="text-warning">~</span> {change.field}{" "}
              <Mono dim className="text-2xs">
                {change.from} → {change.to}
              </Mono>
            </li>
          ))}
        </ul>
      )}
    </Card>
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
