import { useTranslation } from "react-i18next";
import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ClassifyFields, type Ruling } from "@/features/admin/classify-fields";
import {
  blankRuling,
  classificationInput,
  rulingFromSuggestion,
  startRulingFromTool,
} from "@/features/admin/classify-dialog-model";
import type { RulingOffer } from "@/features/admin/classify-offer";
import { RulingLevelField } from "@/features/admin/ruling-level-field";
import { ToolRulings } from "@/features/admin/tool-rulings";
import {
  useClassifyTool,
  type RulingLevel,
  type Tool,
} from "@/features/admin/api";
import { SuggestedRuling } from "@/features/admin/suggested-ruling";
import { problemMessage } from "@/lib/api/problem-message";

const BLANK: Ruling = blankRuling();

/**
 * The Curator's act, and the only way write access enters the platform.
 *
 * It is a dialog rather than an inline control on purpose: promoting a tool is
 * a decision somebody signs, and the reason is recorded next to it.
 */
export function ClassifyDialog({
  tool,
  tools,
  onClose,
  offer = "platform",
}: {
  tool: Tool | null;
  /** The catalogue, so the ruling can name the tool that undoes this one. */
  tools: Tool[];
  onClose: () => void;
  /** Which ruling the caller may record on this tool (NT-019). */
  offer?: RulingOffer;
}) {
  const { t } = useTranslation();
  const classify = useClassifyTool();
  /*
    Reset per tool, and started from what the level being recorded says.

    It once began blank and stayed mounted between tools: a destructive tool
    with only a reason filled in was submitted as `read`, the zero value, and
    one tool's answers could be signed for the next.

    Keyed by the digest as well as the id: a tool whose definition moved is a
    different thing to judge, and carrying the old answers into it is the exact
    mistake the digest exists to prevent.
  */
  const [ruling, setRuling] = useState<Ruling>(BLANK);
  const [judging, setJudging] = useState<string | null>(null);
  const [level, setLevel] = useState<RulingLevel>("platform");
  const identity = tool ? `${tool.toolId}@${tool.digest ?? ""}` : null;

  if (identity !== judging) {
    setJudging(identity);
    const starting = offer === "owner" ? "owner" : "platform";
    setLevel(starting);
    setRuling(tool ? startRulingFromTool(tool, starting) : BLANK);
  }
  // Each level starts from its own ruling, never the one just shown.
  const switchLevel = (next: RulingLevel) => {
    setLevel(next);
    if (tool) setRuling(startRulingFromTool(tool, next));
  };

  if (!tool) return null;

  async function submit() {
    if (!tool) return;
    try {
      const input = classificationInput(tool, ruling, level);
      if (!input) return;
      await classify.mutateAsync(input);
      toast.success(
        t("admin.classified", {
          tool: tool.toolId,
          effect: t(`effect.${ruling.effect}`),
        }),
      );
      onClose();
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="font-mono text-base">
            {tool.toolId}
          </DialogTitle>
          <DialogDescription>{t("admin.recordedInTrail")}</DialogDescription>
        </DialogHeader>

        <ToolRulings tool={tool} canWithdraw={offer !== "owner"} />

        {tool.suggested && (
          <SuggestedRuling
            suggested={tool.suggested}
            onAccept={() => setRuling(rulingFromSuggestion(tool, ruling))}
          />
        )}

        <RulingLevelField offer={offer} level={level} onChange={switchLevel} />

        <ClassifyFields
          ruling={ruling}
          onChange={setRuling}
          tools={tools}
          self={tool.toolId}
        />

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          {/* Refused until an effect is chosen, rather than defaulted to one.
              A disabled button asks the question again; a default answers it
              with whatever the form happened to hold. */}
          <Button
            onClick={() => void submit()}
            disabled={
              classify.isPending ||
              classificationInput(tool, ruling, level) === null
            }
          >
            {t("admin.recordClassification")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
