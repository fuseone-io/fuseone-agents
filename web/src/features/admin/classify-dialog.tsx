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
import { problemMessage } from "@/lib/api/problem-message";

const BLANK: Ruling = {
  effect: "read",
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
  const [ruling, setRuling] = useState<Ruling>(BLANK);
  const classify = useClassifyTool();

  if (!tool) return null;

  async function submit() {
    if (!tool) return;
    try {
      await classify.mutateAsync({ toolId: tool.toolId, ...ruling });
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
          <Button onClick={() => void submit()} disabled={classify.isPending}>
            {t("admin.recordClassification")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
