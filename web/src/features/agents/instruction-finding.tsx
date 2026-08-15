import { useTranslation } from "react-i18next";
import { CircleAlert, ShieldX } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import type { Finding } from "@/features/agents/instruction-lint";

/**
 * A sentence that promises something that will not happen.
 *
 * Inside the block that produced it, never as a banner on the card: a warning
 * at the top leaves somebody hunting for the sentence it is about, and by the
 * third visit they stop reading it at all.
 *
 * Two exits, both real, and which two depends on what is wrong. A tool the
 * platform refuses cannot be fixed from here, so the choice is to delete the
 * sentence or to keep it because it explains the rule to the next reader. A
 * tool the agent simply does not hold needs a checkbox, not an edit, so that
 * is the first exit — and "keep it, it explains" is not offered, because a
 * sentence naming a tool nobody granted is not explaining anything.
 *
 * There is no dismissal without a decision and no third "learn more": a
 * warning nobody has to answer is a warning that is always there.
 */
export function InstructionFinding({
  finding,
  onRemove,
  onKeep,
  onEnable,
}: {
  finding: Finding;
  onRemove: () => void;
  onKeep: () => void;
  onEnable: () => void;
}) {
  const { t } = useTranslation();
  const refused = finding.why === "refused";

  return (
    <div className="mt-2 flex flex-col gap-2 rounded-lg border border-border bg-card p-3">
      <Headline finding={finding} />

      <p className="text-2xs text-muted-foreground">
        {t(refused ? "agents.forbiddingTwiceAges" : "agents.notInThePack")}
      </p>

      <div className="flex items-center gap-2">
        {refused ? (
          <>
            <Exit onClick={onRemove}>{t("agents.removeTheSentence")}</Exit>
            <Exit ghost onClick={onKeep}>
              {t("agents.keepItExplains")}
            </Exit>
          </>
        ) : (
          <>
            <Exit onClick={onEnable}>{t("agents.enableTheTool")}</Exit>
            <Exit ghost onClick={onRemove}>
              {t("agents.removeTheSentence")}
            </Exit>
          </>
        )}
      </div>
    </div>
  );
}

/** The one sentence of fact, which names what decided rather than the outcome. */
function Headline({ finding }: { finding: Finding }) {
  const { t } = useTranslation();

  if (finding.why === "notEnabled") {
    return (
      <p className="flex items-start gap-2 text-xs">
        <CircleAlert className="mt-px size-4 shrink-0 text-warning" aria-hidden />
        <span>
          {t("agents.agentDoesNotHold")}{" "}
          <Mono className="text-2xs">{finding.tool}</Mono>
        </span>
      </p>
    );
  }

  return (
    <p className="flex items-start gap-2 text-xs">
      <ShieldX className="mt-px size-4 shrink-0 text-danger" aria-hidden />
      <span>
        {finding.because ? (
          <>
            {t("agents.policyAlreadyDenies")}{" "}
            <Mono className="text-2xs">{finding.because}</Mono>{" "}
            <Mono className="text-2xs">{finding.tool}</Mono>
          </>
        ) : (
          <>
            {t("agents.ladderAlreadyDenies")}{" "}
            <Mono className="text-2xs">{finding.tool}</Mono>
          </>
        )}
      </span>
    </p>
  );
}

function Exit({
  ghost,
  onClick,
  children,
}: {
  ghost?: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <Button
      type="button"
      variant={ghost ? "ghost" : "outline"}
      size="sm"
      className="h-6 text-2xs"
      onClick={onClick}
    >
      {children}
    </Button>
  );
}
