import type { Control, FieldPath } from "react-hook-form";
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import type { EvidenceInstanceValues } from "@/features/integrations/connectors/evidence-instance-model";

type Field = {
  control: Control<EvidenceInstanceValues>;
  name: FieldPath<EvidenceInstanceValues>;
  label: string;
  description?: string;
  type?: "text" | "number" | "password" | "lines";
};

export function EvidenceTextField({
  field: { control, name, label, description, type = "text" },
}: {
  field: Field;
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            {type === "lines" ? (
              <Textarea
                {...field}
                value={String(field.value ?? "")}
                className="min-h-20 font-mono text-xs"
                spellCheck={false}
              />
            ) : (
              <Input
                {...field}
                value={String(field.value ?? "")}
                type={type}
                autoComplete="off"
                className="font-mono"
              />
            )}
          </FormControl>
          {description && <FormDescription>{description}</FormDescription>}
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
