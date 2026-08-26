import type { Control, FieldPath } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
} from "@/components/ui/form";
import type { MemoryFormValues } from "@/features/memory/memory-form-schema";

export function MemoryInputField({
  control,
  name,
  label,
  description,
  placeholder,
  className,
}: {
  control: Control<MemoryFormValues>;
  name: FieldPath<MemoryFormValues>;
  label: string;
  description?: string;
  placeholder?: string;
  className?: string;
}) {
  const { t } = useTranslation();
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t(label)}</FormLabel>
          <FormControl>
            <Input
              {...field}
              className={className}
              placeholder={placeholder ? t(placeholder) : undefined}
            />
          </FormControl>
          {description && <FormDescription>{t(description)}</FormDescription>}
        </FormItem>
      )}
    />
  );
}

export function MemoryTextareaField({
  control,
  name,
  label,
  description,
  placeholder,
}: {
  control: Control<MemoryFormValues>;
  name: FieldPath<MemoryFormValues>;
  label: string;
  description?: string;
  placeholder?: string;
}) {
  const { t } = useTranslation();
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t(label)}</FormLabel>
          <FormControl>
            <Textarea
              {...field}
              className="min-h-24"
              placeholder={placeholder ? t(placeholder) : undefined}
            />
          </FormControl>
          {description && <FormDescription>{t(description)}</FormDescription>}
        </FormItem>
      )}
    />
  );
}
