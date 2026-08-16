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
import {
  ClassifyFields,
  type Ruling,
} from "@/features/admin/classify-fields";
import { useClassifyTool, type Tool } from "@/features/admin/api";
import { SuggestedRuling } from "@/features/admin/suggested-ruling";
import { problemMessage } from "@/lib/api/problem-message";

const BLANK: Ruling = {
  // Not one of the four. See Ruling: `read` is a permission, so starting
  // there would make the untouched form a grant.
  effect: "",
  untrusted: true,
  reason: "",
  compensatedBy: "",
};

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
}: {
  tool: Tool | null;
  /** The catalogue, so the ruling can name the tool that undoes this one. */
  tools: Tool[];
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const classify = useClassifyTool();
  /*
    Reset per tool, and started from what the tool already says.

    The state used to begin blank and stay mounted between tools, which made
    two failures out of one mistake. Opening "change the ruling" on a
    destructive tool and filling in only the reason submitted `read` — the
    dialog's zero value, not anybody's judgement. And what was typed for one
    tool was still there for the next, so a decision could be signed with
    another tool's answers.

    Keyed by the digest as well as the id: a tool whose definition moved is a
    different thing to judge, and carrying the old answers into it is the exact
    mistake the digest exists to prevent.
  */
  const [ruling, setRuling] = useState<Ruling>(BLANK);
  const [judging, setJudging] = useState<string | null>(null);
  const identity = tool ? `${tool.toolId}@${tool.digest ?? ""}` : null;

  if (identity !== judging) {
    setJudging(identity);
    setRuling(tool ? startFrom(tool) : BLANK);
  }

  if (!tool) return null;

  async function submit() {
    if (!tool) return;
    try {
      if (ruling.effect === "") return;
      await classify.mutateAsync({
        toolId: tool.toolId,
        digest: tool.digest,
        ...ruling,
        effect: ruling.effect,
      });
      toast.success(
        t("admin.classified", {
          tool: tool.toolId,
          effect: t(`effect.${ruling.effect}`),
        }),
      );
      onClose();
    } catch (error) {
      toast.error(
        problemMessage(error, t),
      );
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

        {tool.suggested && (
          <SuggestedRuling
            suggested={tool.suggested}
            onAccept={() =>
              setRuling({
                // No fallback. The contract makes this required, so the
                // only way `??` fires is a suggestion that arrived without
                // one — and it would fire into `read`, which is the value
                // this dialog has just stopped defaulting to everywhere else.
                // An empty effect leaves the form unanswered, which is the
                // honest reading of a suggestion that suggests nothing.
                effect: tool.suggested?.effect ?? "",
                untrusted: tool.suggested?.untrusted ?? true,
                compensatedBy: tool.suggested?.compensatedBy ?? "",
                // The reason stays theirs. Accepting a suggestion is still a
                // decision somebody signs, and signing somebody else's
                // sentence is not the same as writing one.
                reason: "",
              })
            }
          />
        )}

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
            disabled={classify.isPending || ruling.effect === ""}
          >
            {t("admin.recordClassification")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/*
startFrom is what a ruling opens on.

A tool nobody has judged opens blank: there is nothing to carry forward, and a
pre-filled effect would be the platform answering for the Curator.

A tool already ruled on opens on its ruling. The act is "change this", and
starting from the zero value makes the safest-looking answer — `read` — the one
a distracted person submits by touching nothing.
*/
function startFrom(tool: Tool): Ruling {
  if (tool.effect === "unknown") return BLANK;
  return {
    effect: tool.effect,
    untrusted: tool.untrusted,
    reason: "",
    compensatedBy: tool.compensatedBy ?? "",
  };
}
