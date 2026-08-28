import type { Control } from "react-hook-form";
import { useTranslation } from "react-i18next";
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
} from "@/components/ui/form";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import type { MemoryFormValues } from "@/features/memory/memory-form-schema";

/**
 * Who reads this memory.
 *
 * Both options visible rather than a select, because shared memory is what
 * every agent in the scope recalls and that is not a choice to make from a
 * collapsed list. It used to be an agent field left blank, which meant the
 * widest reach in the platform was one forgotten input away.
 *
 * The agent itself is not asked for: it is the one whose run the evidence
 * names, and the server reads it from the ledger.
 */
export function MemoryNamespaceField({
  control,
}: {
  control: Control<MemoryFormValues>;
}) {
  const { t } = useTranslation();
  return (
    <FormField
      control={control}
      name="namespace"
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t("memory.namespace")}</FormLabel>
          <FormControl>
            <RadioGroup
              onValueChange={field.onChange}
              value={field.value}
              className="gap-2"
            >
              {(["agent", "shared"] as const).map((choice) => (
                <div key={choice} className="flex items-center gap-2">
                  <RadioGroupItem value={choice} id={`namespace-${choice}`} />
                  <Label
                    htmlFor={`namespace-${choice}`}
                    className="font-normal"
                  >
                    {t(`memory.namespace_${choice}`)}
                  </Label>
                </div>
              ))}
            </RadioGroup>
          </FormControl>
          <FormDescription>{t("memory.namespaceHint")}</FormDescription>
        </FormItem>
      )}
    />
  );
}
