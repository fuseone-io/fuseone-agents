import { useTranslation } from "react-i18next";
import { Eye, GitCompare, Pencil } from "lucide-react";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

export type InstructionsView = "write" | "read" | "diff";

/**
 * The three ways of looking at one instruction.
 *
 * Writing is the default. Reading it as the agent reads it is not optional —
 * it is the proof the editor invents nothing, and what a responder copies
 * during an incident.
 *
 * What changed appears only when something did. A segment that is present and
 * empty teaches people it is never worth pressing, and by then it is the one
 * they needed.
 */
export function InstructionsViewTabs({
  view,
  onChange,
  changed,
}: {
  view: InstructionsView;
  onChange: (view: InstructionsView) => void;
  /** Whether the prose differs from the published version. */
  changed: boolean;
}) {
  const { t } = useTranslation();

  return (
    <Tabs value={view} onValueChange={(next) => onChange(next as InstructionsView)}>
      <TabsList className="h-8">
        <TabsTrigger value="write">
          <Pencil className="size-3.5" aria-hidden />
          {t("agents.writeIt")}
        </TabsTrigger>
        <TabsTrigger value="read">
          <Eye className="size-3.5" aria-hidden />
          {t("agents.readAsAgent")}
        </TabsTrigger>
        {changed && (
          <TabsTrigger value="diff">
            <GitCompare className="size-3.5" aria-hidden />
            {t("agents.whatChanged")}
          </TabsTrigger>
        )}
      </TabsList>
    </Tabs>
  );
}
