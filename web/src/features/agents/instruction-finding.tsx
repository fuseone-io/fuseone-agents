import { useTranslation } from "react-i18next";
import { CircleAlert, ShieldX } from "lucide-react";
import { Mono } from "@/components/shared/mono";
import type { Finding } from "@/features/agents/instruction-lint";

/** The one sentence of fact, which names what decided rather than the outcome. */
export function FindingHeadline({ finding }: { finding: Finding }) {
  const { t } = useTranslation();

  if (finding.why === "notEnabled") {
    return (
      <p className="flex items-start gap-2 text-xs">
        <CircleAlert
          className="mt-px size-4 shrink-0 text-warning"
          aria-hidden
        />
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
