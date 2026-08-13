import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useSetStage, type Stage } from "@/features/agents/stage-api";
import { problemMessage } from "@/lib/api/problem-message";

const ORDER: Stage[] = ["draft", "copilot", "autonomous"];

/**
 * How far this agent is trusted, and the one control that changes it.
 *
 * The consequence of each choice is written beside it rather than left to be
 * discovered: promoting to autonomous is the moment an agent starts doing
 * things nobody will be asked about, and a dropdown that said only
 * "autonomous" would be hiding that behind a word.
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

  const change = (next: Stage) =>
    set.mutate(next, {
      onSuccess: () => toast.success(t(`stage.moved.${next}`)),
      // The server refuses a promotion out of Draft with nothing simulated,
      // and its sentence is the useful one.
      onError: (error) =>
        toast.error(t("stage.failed"), {
          description: problemMessage(error, t),
        }),
    });

  return (
    <Select
      value={stage ?? "draft"}
      onValueChange={(next) => change(next as Stage)}
      disabled={set.isPending}
    >
      <SelectTrigger className="!h-8 w-[190px]" aria-label={t("stage.label")}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {ORDER.map((option) => (
          <SelectItem key={option} value={option}>
            <span className="flex flex-col items-start">
              <span>{t(`stage.${option}`)}</span>
              <span className="text-xs text-muted-foreground">
                {t(`stage.means.${option}`)}
              </span>
            </span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
