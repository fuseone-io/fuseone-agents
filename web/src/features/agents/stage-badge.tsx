import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import type { Stage } from "@/features/agents/stage-api";

/** What each stage means, said rather than coloured: a draft that only looked
 *  grey would be an agent somebody expects to be working. */
const STAGES: Record<
  Stage,
  { label: string; variant: "outline" | "secondary" | "destructive" }
> = {
  draft: { label: "stage.draft", variant: "destructive" },
  copilot: { label: "stage.copilot", variant: "outline" },
  autonomous: { label: "stage.autonomous", variant: "secondary" },
};

export function StageBadge({ stage }: { stage: Stage | undefined }) {
  const { t } = useTranslation();
  // Absent reads as draft, which is what the platform assumes. A client that
  // showed nothing would be hiding the reason an agent is not running.
  const shown = STAGES[stage ?? "draft"];

  return <Badge variant={shown.variant}>{t(shown.label)}</Badge>;
}
