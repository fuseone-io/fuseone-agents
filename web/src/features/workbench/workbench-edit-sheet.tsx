import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { SheetClose } from "@/components/ui/sheet";
import {
  PropertiesSheet,
  PropertiesSheetBody,
  PropertiesSheetFooter,
} from "@/components/shared/properties-sheet";
import { WorkbenchFieldPart, type FieldPart } from "@/features/workbench/workbench-field-part";
import type { AgentDefinition } from "@/lib/api/client";

type Editing = {
  agentId: string;
  local: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
};

/**
 * A part of the draft's manual controls, in a side panel over the workbench.
 * Closing it keeps every change on the panel's local copy, as the header's
 * save count shows; nothing is written until the person saves.
 */
export function WorkbenchEditSheet({
  part,
  onClose,
  editing,
}: {
  part: FieldPart | undefined;
  onClose: () => void;
  editing: Editing;
}) {
  const { t } = useTranslation();
  return (
    <PropertiesSheet
      open={part !== undefined}
      onOpenChange={(open) => !open && onClose()}
      title={part ? t(`workbench.parts.${part}`) : ""}
      description={t("workbench.editNote")}
    >
      <PropertiesSheetBody className="flex flex-col gap-6">
        {part && <WorkbenchFieldPart part={part} {...editing} />}
      </PropertiesSheetBody>
      <PropertiesSheetFooter>
        <SheetClose asChild>
          <Button variant="outline">{t("workbench.done")}</Button>
        </SheetClose>
      </PropertiesSheetFooter>
    </PropertiesSheet>
  );
}
