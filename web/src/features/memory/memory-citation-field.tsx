import type { Control } from "react-hook-form";
import { useTranslation } from "react-i18next";
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
} from "@/components/ui/form";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import type { MemoryFormValues } from "@/features/memory/memory-form-schema";
import { FINAL_ANSWER, type Citation } from "@/features/runs/run-citations";

/**
 * Which of a run's outputs the memory cites, when the run published more than
 * one.
 *
 * A choice among names the ledger recorded, never a box to type one into. Every
 * option here resolves; a typed one only might, and the person typing has no way
 * to find out except by being refused.
 *
 * Absent when the run produced a single citable output, which is the ordinary
 * case: asking somebody to choose between one thing is asking them to confirm
 * that the platform can count.
 */
export function MemoryCitationField({
  control,
  citations,
}: {
  control: Control<MemoryFormValues>;
  citations: Citation[];
}) {
  const { t } = useTranslation();
  if (citations.length < 2) return null;
  return (
    <FormField
      control={control}
      name="evidenceArtifact"
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t("memory.whichOutput")}</FormLabel>
          <FormControl>
            <RadioGroup
              onValueChange={field.onChange}
              value={field.value}
              className="gap-2"
            >
              {citations.map((citation) => (
                <div key={citation.artifact} className="flex items-center gap-2">
                  <RadioGroupItem
                    value={citation.artifact}
                    id={`artifact-${citation.artifact}`}
                  />
                  <Label
                    htmlFor={`artifact-${citation.artifact}`}
                    className="font-mono font-normal"
                  >
                    {citation.artifact === FINAL_ANSWER
                      ? t("memory.closingAnswer")
                      : citation.artifact}
                  </Label>
                </div>
              ))}
            </RadioGroup>
          </FormControl>
        </FormItem>
      )}
    />
  );
}
