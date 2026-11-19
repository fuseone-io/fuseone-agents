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
import { cn } from "@/lib/utils";

type Editing = {
  agentId: string;
  local: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
};

/*
 * Wider than a properties form: the steps' flow and the tool catalogue are a canvas and a
 * table, and at the 720px every admin form uses, the flow showed one card and the tools
 * table cut every name.
 */
const WORKBENCH_SHEET_WIDTH = "lg:max-w-[min(1120px,88vw)] xl:max-w-[min(1320px,88vw)]";

/*
 * Parts whose sections stack and the panel scrolls. In a flex column of fixed height the
 * sections would otherwise shrink below their content and cut it, as the governance cards
 * did. The tools and the steps fill the panel and scroll inside themselves instead.
 */
const SCROLLED_PARTS: ReadonlySet<FieldPart> = new Set(["identity", "governance"]);

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
      className={WORKBENCH_SHEET_WIDTH}
    >
      <PropertiesSheetBody className={cn("flex flex-col gap-6", part && SCROLLED_PARTS.has(part) && "*:shrink-0")}>
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
