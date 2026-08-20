import type { Control } from "react-hook-form";
import { Input } from "@/components/ui/input";
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
} from "@/components/ui/form";
import type {
  PriceFormValues,
  PriceRateName,
} from "@/features/admin/price-form-model";

export function PriceRateField({
  control,
  name,
  label,
}: {
  control: Control<PriceFormValues>;
  name: PriceRateName;
  label: string;
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field, fieldState }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <Input
              {...field}
              inputMode="decimal"
              className="font-mono"
              placeholder="0"
              aria-invalid={fieldState.invalid || undefined}
            />
          </FormControl>
        </FormItem>
      )}
    />
  );
}
