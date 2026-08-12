import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Upload } from "lucide-react";
import { toast } from "sonner";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Panel } from "@/components/shared/panel";
import { cn } from "@/lib/utils";
import { useStartSimulation } from "@/features/agents/simulation-api";
import { countCases } from "@/features/agents/simulation-tally";

/**
 * The set of occurrences to replay, and the one button that starts it.
 *
 * Uploaded rather than fetched from the systems themselves: a connector
 * reading the last fifty tickets would end the property that makes this
 * defensible — that authoring never touches production — and would be an
 * integration project per customer besides (PRD N4).
 */
export function SimulationStart({
  agentId,
  onStarted,
}: {
  agentId: string;
  onStarted: (simulationId: string) => void;
}) {
  const { t } = useTranslation();
  const [cases, setCases] = useState("");
  const start = useStartSimulation(agentId);
  const count = countCases(cases);

  const submit = () =>
    start.mutate(cases, {
      onSuccess: (accepted) => onStarted(accepted.id),
      // The server refuses the whole file and names the line. Shown as it
      // came, because "invalid file" would leave the author guessing which of
      // fifty lines to fix.
      onError: (error) =>
        toast.error(t("simulation.startFailed"), {
          description: error instanceof Error ? error.message : undefined,
        }),
    });

  return (
    <Panel title={t("simulation.setTitle")}>
      <div className="flex max-w-3xl flex-col gap-4">
        <p className="text-sm text-muted-foreground">
          {t("simulation.setHelp")}
        </p>

        {/* The native control names its own button, in the browser's language
            and not the console's — so it is the label that is visible and the
            input that is only reachable. Hidden, never removed: it is still
            what the keyboard tabs to and what opens the picker. */}
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
          // sr-only alone leaves the component's own w-full standing, and an
          // absolutely positioned full-width input hangs off the page. size-px
          // is in the same merge group, so it wins rather than fights.
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
          <p className="text-xs text-muted-foreground">
            {t("simulation.casesCount", { count })}
          </p>
        </div>

        <Button
          className="self-start"
          disabled={count === 0 || start.isPending}
          onClick={submit}
        >
          {t("simulation.start")}
        </Button>
      </div>
    </Panel>
  );
}
