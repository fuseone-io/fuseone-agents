import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import {
  Brain,
  CircleDollarSign,
  ListPlus,
  MessageSquareOff,
  Play,
  Plus,
  ShieldCheck,
  Unplug,
  Upload,
  UserRoundCheck,
} from "lucide-react";
import { toast } from "sonner";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import { useStartSimulation } from "@/features/agents/simulation-api";
import { countCases } from "@/features/agents/simulation-tally";
import { SimulationReadinessNotice } from "@/features/agents/simulation-readiness";
import { simulationReadiness } from "@/features/agents/simulation-readiness-state";
import { problemMessage } from "@/lib/api/problem-message";
import type { Agent } from "@/lib/api/client";

type Source = "corpus" | "write" | "json";

interface WrittenFields {
  subject: string;
  system: string;
  origin: string;
  message: string;
}

const FACTS = [
  { icon: Brain, key: "simulation.factThinks", tone: "text-text-accent" },
  { icon: Unplug, key: "simulation.factToolsDry", tone: "text-muted-foreground" },
  {
    icon: MessageSquareOff,
    key: "simulation.factReplyHeld",
    tone: "text-success",
  },
  {
    icon: CircleDollarSign,
    key: "simulation.factNoCharge",
    tone: "text-muted-foreground",
  },
  {
    icon: UserRoundCheck,
    key: "simulation.factNoApproval",
    tone: "text-muted-foreground",
  },
] as const;

/**
 * The set of situations to rehearse, and the one button that starts it.
 *
 * The old screen made JSONL the default interface. JSONL remains available,
 * but the normal path now names what a person is choosing: saved situations,
 * a hand-written situation, or pasted data.
 */
export function SimulationStart({
  agentId,
  agent,
  agentLoading,
  agentError,
  onRetryAgent,
  onStarted,
}: {
  agentId: string;
  agent?: Agent;
  agentLoading?: boolean;
  agentError?: Error | null;
  onRetryAgent?: () => void;
  onStarted: (simulationId: string) => void;
}) {
  const { t } = useTranslation();
  const [source, setSource] = useState<Source>("corpus");
  const [cases, setCases] = useState("");
  const [written, setWritten] = useState<string[]>([]);
  const [fields, setFields] = useState<WrittenFields>({
    subject: "",
    system: "",
    origin: "",
    message: "",
  });
  const start = useStartSimulation(agentId);
  const pastedCount = countCases(cases);
  const chosenCount = source === "write" ? written.length : pastedCount;
  const readiness = simulationReadiness({ agent, agentLoading, agentError, t });
  const manualReady = fields.subject.trim() !== "" || fields.message.trim() !== "";
  const blocksStart = readiness?.blocksStart || start.isPending;
  const canStart =
    !blocksStart && (source === "corpus" || (source === "write" ? written.length > 0 : pastedCount > 0));

  const startWith = (input: { cases?: string; corpus?: boolean }) =>
    start.mutate(input, {
      onSuccess: (accepted) => onStarted(accepted.id),
      // The server refuses the whole file and names the line. Shown as it
      // came, because "invalid file" would leave the author guessing which of
      // fifty lines to fix.
      onError: (error) =>
        toast.error(t("simulation.startFailed"), {
          description: problemMessage(error, t),
        }),
    });

  const submit = () => {
    if (source === "corpus") return startWith({ corpus: true });
    if (source === "write") return startWith({ cases: written.join("\n") });
    return startWith({ cases });
  };

  const addWritten = () => {
    if (!manualReady) return;
    setWritten((current) => [...current, JSON.stringify(compact(fields))]);
    setFields({ subject: "", system: "", origin: "", message: "" });
  };

  const clear = () => {
    if (source === "write") setWritten([]);
    if (source === "json") setCases("");
  };

  return (
    <div className="grid min-w-0 gap-5 lg:grid-cols-[minmax(0,1fr)_316px] lg:items-start">
      <main className="flex min-w-0 flex-col gap-4">
        <StepCard
          number={1}
          title={t("simulation.pickTitle")}
          hint={t("simulation.pickHint")}
          action={
            <div className="flex min-w-0 overflow-x-auto rounded-md border bg-muted p-1">
              {(["corpus", "write", "json"] as const).map((one) => (
                <SourceButton
                  key={one}
                  active={source === one}
                  onClick={() => setSource(one)}
                >
                  {t(`simulation.source.${one}`)}
                </SourceButton>
              ))}
            </div>
          }
        >
          {readiness && (
            <SimulationReadinessNotice
              readiness={readiness}
              agentId={agentId}
              onRetry={onRetryAgent}
            />
          )}

          {source === "corpus" && (
            <div className="rounded-lg border bg-muted p-4">
              <div className="flex items-start gap-3">
                <ShieldCheck
                  className="mt-0.5 size-4 shrink-0 text-text-accent"
                  aria-hidden
                />
                <div className="min-w-0 space-y-1">
                  <p className="text-sm font-medium">
                    {t("simulation.savedSituationsTitle")}
                  </p>
                  <p className="text-sm text-muted-foreground">
                    {t("simulation.savedSituationsBody")}
                  </p>
                </div>
              </div>
            </div>
          )}

          {source === "write" && (
            <div className="flex max-w-3xl flex-col gap-3">
              <p className="text-sm text-muted-foreground">
                {t("simulation.writeHelp")}
              </p>
              <div className="grid min-w-0 gap-3 sm:grid-cols-3">
                <TextField
                  id="sim-subject"
                  label={t("simulation.whatArrived")}
                  hint={t("simulation.whatArrivedHint")}
                  value={fields.subject}
                  placeholder={t("simulation.whatArrivedPlaceholder")}
                  onChange={(subject) =>
                    setFields((current) => ({ ...current, subject }))
                  }
                />
                <TextField
                  id="sim-system"
                  label={t("simulation.whichSystem")}
                  hint={t("simulation.optional")}
                  value={fields.system}
                  placeholder={t("simulation.whichSystemPlaceholder")}
                  onChange={(system) =>
                    setFields((current) => ({ ...current, system }))
                  }
                />
                <TextField
                  id="sim-origin"
                  label={t("simulation.whereFrom")}
                  hint={t("simulation.optional")}
                  value={fields.origin}
                  placeholder={t("simulation.whereFromPlaceholder")}
                  onChange={(origin) =>
                    setFields((current) => ({ ...current, origin }))
                  }
                />
              </div>
              <div className="flex min-w-0 flex-col gap-1.5">
                <Label htmlFor="sim-message">{t("simulation.messageItself")}</Label>
                <Textarea
                  id="sim-message"
                  value={fields.message}
                  onChange={(event) =>
                    setFields((current) => ({
                      ...current,
                      message: event.target.value,
                    }))
                  }
                  placeholder={t("simulation.messagePlaceholder")}
                  className="min-h-28"
                />
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={!manualReady}
                  onClick={addWritten}
                >
                  <Plus className="size-4" aria-hidden />
                  {t("simulation.addSituation")}
                </Button>
                <span className="text-xs text-muted-foreground">
                  {t("simulation.addSituationHint")}
                </span>
              </div>
            </div>
          )}

          {source === "json" && (
            <div className="flex max-w-3xl flex-col gap-4">
              <p className="text-sm text-muted-foreground">
                {t("simulation.jsonHelp")}
              </p>
              <Label
                htmlFor="sim-file"
                className={cn(
                  buttonVariants({ variant: "outline", size: "sm" }),
                  "w-fit cursor-pointer",
                )}
              >
                <Upload className="size-4" aria-hidden />
                {t("simulation.fileLabel")}
              </Label>
              <Input
                id="sim-file"
                type="file"
                accept=".jsonl,.ndjson,.json,.txt"
                className="sr-only size-px"
                onChange={(e) => {
                  const file = e.target.files?.[0];
                  if (file) void file.text().then(setCases);
                }}
              />

              <div className="flex flex-col gap-2">
                <Label htmlFor="sim-cases">{t("simulation.casesLabel")}</Label>
                <Textarea
                  id="sim-cases"
                  value={cases}
                  onChange={(e) => setCases(e.target.value)}
                  placeholder={t("simulation.casesPlaceholder")}
                  className="min-h-40 font-mono text-xs"
                  spellCheck={false}
                />
                <p className="font-mono text-xs text-muted-foreground">
                  {t("simulation.linesRead", { count: pastedCount })}
                </p>
              </div>
            </div>
          )}
        </StepCard>

        <StepCard
          number={2}
          active={source === "corpus" || chosenCount > 0}
          title={t("simulation.rehearseTitle")}
          hint={
            source === "corpus"
              ? t("simulation.readySaved")
              : chosenCount === 0
                ? t("simulation.notReady")
                : t("simulation.readyN", { count: chosenCount })
          }
          action={
            <div className="flex shrink-0 items-center gap-2">
              {source !== "corpus" && chosenCount > 0 && (
                <Button variant="ghost" size="sm" onClick={clear}>
                  {t("simulation.clear")}
                </Button>
              )}
              <Button disabled={!canStart} onClick={submit}>
                <Play className="size-4" aria-hidden />
                {source === "corpus"
                  ? t("simulation.rehearseSaved")
                  : t("simulation.rehearseN", { count: chosenCount })}
              </Button>
            </div>
          }
        >
          {source !== "corpus" && chosenCount === 0 ? (
            <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed px-6 py-10 text-center">
              <ListPlus className="size-5 text-muted-foreground" aria-hidden />
              <p className="text-sm font-medium">{t("simulation.noSituation")}</p>
              <p className="max-w-md text-sm text-muted-foreground">
                {t("simulation.noSituationHint")}
              </p>
            </div>
          ) : (
            <ul className="divide-y overflow-hidden rounded-lg border">
              {source === "corpus" ? (
                <SituationRow
                  title={t("simulation.savedSituationsTitle")}
                  subtitle={t("simulation.savedSituationsShort")}
                  state={t("simulation.ready")}
                />
              ) : source === "write" ? (
                written.map((line, index) => (
                  <SituationRow
                    key={`${line}-${index}`}
                    title={titleOf(line, index, t)}
                    subtitle={t("simulation.handWritten")}
                    state={t("simulation.notRehearsed")}
                  />
                ))
              ) : (
                <SituationRow
                  title={t("simulation.pastedSituations", { count: pastedCount })}
                  subtitle={t("simulation.serverValidates")}
                  state={t("simulation.notRehearsed")}
                />
              )}
            </ul>
          )}
        </StepCard>
      </main>

      <aside className="flex min-w-0 flex-col gap-4 lg:sticky lg:top-0">
        <RailCard title={t("simulation.whatRehearsalDoes")}>
          <ul className="divide-y">
            {FACTS.map(({ icon: Icon, key, tone }) => (
              <li key={key} className="flex items-start gap-3 py-2.5">
                <Icon className={cn("mt-0.5 size-3.5 shrink-0", tone)} aria-hidden />
                <span className="min-w-0 text-sm text-muted-foreground">
                  {t(key)}
                </span>
              </li>
            ))}
          </ul>
          <p className="border-t pt-3 text-xs text-muted-foreground">
            {t("simulation.rehearsalNotHistory")}
          </p>
        </RailCard>
      </aside>
    </div>
  );
}

function compact(fields: WrittenFields) {
  return Object.fromEntries(
    [
      ["subject", fields.subject.trim()],
      ["system", fields.system.trim()],
      ["source", fields.origin.trim()],
      ["message", fields.message.trim()],
    ].filter(([, value]) => value !== ""),
  );
}

function titleOf(line: string, index: number, t: TFunction) {
  try {
    const parsed = JSON.parse(line) as Record<string, unknown>;
    const subject = parsed.subject;
    if (typeof subject === "string" && subject.trim() !== "") return subject;
  } catch {
    // This was created by this component, so parse failure means the author
    // edited the source in devtools; falling back is enough for display.
  }
  return t("simulation.situationNumber", { n: index + 1 });
}

function StepCard({
  number,
  title,
  hint,
  action,
  active = true,
  children,
}: {
  number: number;
  title: string;
  hint: string;
  action?: ReactNode;
  active?: boolean;
  children: ReactNode;
}) {
  return (
    <section className="overflow-hidden rounded-xl border bg-card shadow-sm">
      <header className="flex min-w-0 flex-wrap items-center gap-3 border-b px-4 py-3">
        <span
          className={cn(
            "grid size-5 shrink-0 place-items-center rounded-full font-mono text-[11px]",
            active
              ? "bg-surface-accent text-text-accent"
              : "bg-muted text-muted-foreground",
          )}
        >
          {number}
        </span>
        <div className="min-w-0 flex-1">
          <h2 className="text-sm font-medium">{title}</h2>
          <p className="truncate text-xs text-muted-foreground">{hint}</p>
        </div>
        {action}
      </header>
      <div className="flex min-w-0 flex-col gap-4 p-4">{children}</div>
    </section>
  );
}

function SourceButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "h-7 shrink-0 rounded px-3 text-xs font-medium",
        active
          ? "border border-border bg-card text-foreground shadow-sm"
          : "text-muted-foreground hover:text-foreground",
      )}
    >
      {children}
    </button>
  );
}

function TextField({
  id,
  label,
  hint,
  value,
  placeholder,
  onChange,
}: {
  id: string;
  label: string;
  hint: string;
  value: string;
  placeholder: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <Label htmlFor={id}>
        {label} <span className="text-xs text-muted-foreground">{hint}</span>
      </Label>
      <Input
        id={id}
        value={value}
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value)}
      />
    </div>
  );
}

function SituationRow({
  title,
  subtitle,
  state,
}: {
  title: string;
  subtitle: string;
  state: string;
}) {
  return (
    <li className="flex min-w-0 items-center gap-3 px-4 py-3">
      <span className="grid size-5 shrink-0 place-items-center rounded-full bg-muted text-muted-foreground">
        <ListPlus className="size-3" aria-hidden />
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">{title}</p>
        <p className="truncate text-xs text-muted-foreground">{subtitle}</p>
      </div>
      <span className="shrink-0 text-xs text-muted-foreground">{state}</span>
    </li>
  );
}

function RailCard({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <section className="flex min-w-0 flex-col gap-3 rounded-xl border bg-card p-4 shadow-sm">
      <h2 className="text-2xs uppercase tracking-label text-muted-foreground">
        {title}
      </h2>
      {children}
    </section>
  );
}
