import { useTranslation } from "react-i18next";
import { FileText } from "lucide-react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Section } from "@/features/policies/section";
import { Textarea } from "@/components/ui/textarea";
import { AgentFlowEditor } from "@/features/agents/agent-flow-editor";
import type { AgentDefinition, Policy, Tool } from "@/lib/api/client";

/**
 * What the agent is told to do, written and read.
 *
 * One field with two views rather than two fields. The prose is the single
 * account of the process — it is what the model receives and what an auditor
 * reads to understand a run (FU-08) — and the steps are that same account read
 * as the stages it describes.
 *
 * They were a separate section underneath, which made them look like a second
 * thing to fill in: an author who had just written their process in words
 * found an empty form below asking for it again. Both views live here now, so
 * the second is visibly about the first.
 */
export function AgentInstructionsField({
  draft,
  patch,
  catalogue,
  policies,
}: {
  draft: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
  catalogue: Tool[];
  policies: Policy[];
}) {
  const { t } = useTranslation();
  const declared = draft.steps ?? [];

  return (
    <Section
      icon={FileText}
      title={t("agents.instructions")}
      hint={t("agents.instructionsHint")}
    >
      <Tabs defaultValue="write">
        <div className="flex items-center gap-3">
          <TabsList className="ml-auto h-8">
            <TabsTrigger value="write">{t("agents.writeIt")}</TabsTrigger>
            <TabsTrigger value="steps">
              {t("agents.asSteps", { count: declared.length })}
            </TabsTrigger>
          </TabsList>
        </div>

        <TabsContent value="write" className="flex flex-col gap-1.5">
          <Textarea
            id="agent-instructions"
            rows={8}
            value={draft.instructions}
            onChange={(e) => patch({ instructions: e.target.value })}
            className="font-mono text-xs"
            placeholder={t("agents.instructionsPlaceholder")}
            aria-label={t("agents.instructions")}
          />
          <p className="text-xs text-muted-foreground">
            {t("agents.instructionsLength", {
              count: draft.instructions.trim().length,
            })}
          </p>
        </TabsContent>

        <TabsContent value="steps">
          <AgentFlowEditor
            draft={draft}
            patch={patch}
            catalogue={catalogue}
            policies={policies}
          />
        </TabsContent>
      </Tabs>
    </Section>
  );
}
