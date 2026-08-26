import { Brain } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Section, Labelled } from "@/features/policies/section";
import {
  DEFAULT_MIN_OBSERVATIONS,
  DEFAULT_TTL_DAYS,
  MAX_MIN_OBSERVATIONS,
  MAX_TTL_DAYS,
  boundedInt,
  normaliseLearning,
  type LearningMode,
} from "@/features/agents/agent-memory-learning-model";
import type { AgentDefinition, MemoryLearningPolicy } from "@/lib/api/client";

export function AgentMemoryLearningSection({
  draft,
  patch,
}: {
  draft: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
}) {
  const { t } = useTranslation();
  const policy = normaliseLearning(draft.memoryLearning);

  const setMode = (mode: LearningMode) =>
    patch({
      memoryLearning:
        mode === "off"
          ? undefined
          : { ...policy, mode },
    });
  const setNumber = (over: Partial<MemoryLearningPolicy>) =>
    patch({ memoryLearning: { ...policy, ...over } });

  return (
    <Section
      icon={Brain}
      title={t("agents.memoryLearning")}
      hint={t("agents.memoryLearningHint")}
    >
      <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_150px_150px]">
        <Labelled
          label={t("agents.memoryLearningMode")}
          htmlFor="memory-learning-mode"
        >
          <Select
            value={policy.mode}
            onValueChange={(mode) => setMode(mode as LearningMode)}
          >
            <SelectTrigger id="memory-learning-mode">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="off">
                {t("agents.memoryLearningModeOff")}
              </SelectItem>
              <SelectItem value="review">
                {t("agents.memoryLearningModeReview")}
              </SelectItem>
              <SelectItem value="auto_confirm">
                {t("agents.memoryLearningModeAutoConfirm")}
              </SelectItem>
            </SelectContent>
          </Select>
        </Labelled>
        <Labelled
          label={t("agents.memoryLearningMin")}
          htmlFor="memory-learning-min"
        >
          <Input
            id="memory-learning-min"
            type="number"
            min={2}
            max={MAX_MIN_OBSERVATIONS}
            disabled={policy.mode !== "auto_confirm"}
            value={policy.minObservations ?? DEFAULT_MIN_OBSERVATIONS}
            onChange={(event) =>
              setNumber({
                minObservations: boundedInt(
                  event.target.value,
                  2,
                  MAX_MIN_OBSERVATIONS,
                ),
              })
            }
            className="font-mono"
          />
        </Labelled>
        <Labelled
          label={t("agents.memoryLearningTTL")}
          htmlFor="memory-learning-ttl"
        >
          <Input
            id="memory-learning-ttl"
            type="number"
            min={1}
            max={MAX_TTL_DAYS}
            disabled={policy.mode === "off"}
            value={policy.ttlDays ?? DEFAULT_TTL_DAYS}
            onChange={(event) =>
              setNumber({
                ttlDays: boundedInt(event.target.value, 1, MAX_TTL_DAYS),
              })
            }
            className="font-mono"
          />
        </Labelled>
      </div>

      <p className="text-xs text-muted-foreground">
        {t(`agents.memoryLearningExplains.${policy.mode}`)}
      </p>
    </Section>
  );
}
