import { useTranslation } from "react-i18next";
import { Bot, Pencil, UserRoundCheck } from "lucide-react";
import { toast } from "sonner";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useSetStage, type Stage } from "@/features/agents/stage-api";
import { problemMessage } from "@/lib/api/problem-message";

const ORDER: Stage[] = ["draft", "copilot", "autonomous"];
const ICONS = {
  draft: Pencil,
  copilot: UserRoundCheck,
  autonomous: Bot,
} satisfies Record<Stage, typeof Bot>;

// Written out so the guard that checks every key exists can see them.
const NAMES: Record<Stage, string> = {
  draft: "stage.draft",
  copilot: "stage.copilot",
  autonomous: "stage.autonomous",
};

const MOVED: Record<Stage, string> = {
  draft: "stage.moved.draft",
  copilot: "stage.moved.copilot",
  autonomous: "stage.moved.autonomous",
};

/**
 * How far this agent is trusted, as a segmented control.
 *
 * It was a dropdown carrying the consequence of each choice inside the option,
 * which made the trigger two lines tall and knocked every other control in the
 * row out of line. The consequence did not stop being worth saying — it moved
 * out, to one line beside the control, where it is legible without opening
 * anything.
 *
 * Three choices that are always three: a menu that has to be opened to find
 * out what is in it is the wrong shape for a closed set this small.
 */
export function StageControl({
  agentId,
  stage,
}: {
  agentId: string;
  stage: Stage | undefined;
}) {
  const { t } = useTranslation();
  const set = useSetStage(agentId);
  const current = stage ?? "draft";

  const change = (next: string) =>
    set.mutate(next as Stage, {
      onSuccess: () => toast.success(t(MOVED[next as Stage])),
      // The server refuses a promotion out of Draft with nothing simulated,
      // and its sentence is the useful one.
      onError: (error) =>
        toast.error(t("stage.failed"), {
          description: problemMessage(error, t),
        }),
    });

  return (
    <Tabs value={current} onValueChange={change}>
      <TabsList className="h-8" aria-label={t("stage.label")}>
        {ORDER.map((option) => {
          const Icon = ICONS[option];
          return (
            <TabsTrigger key={option} value={option} disabled={set.isPending}>
              <Icon aria-hidden />
              {t(NAMES[option])}
            </TabsTrigger>
          );
        })}
      </TabsList>
    </Tabs>
  );
}
