import type { ReactNode } from "react";
import type { UseFormReturn } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Form } from "@/components/ui/form";
import { EvidenceFields } from "@/features/memory/memory-evidence-fields";
import { MemoryFactFields } from "@/features/memory/memory-fact-fields";
import { MemoryInputField } from "@/features/memory/memory-form-fields";
import { MemoryNamespaceField } from "@/features/memory/memory-namespace-field";
import { MemoryScopeField } from "@/features/memory/memory-scope-field";
import type { MemoryFormValues } from "@/features/memory/memory-form-schema";

export function MemoryCreateForm({
  form,
  isPending,
  matchNotice,
  onSubmit,
}: {
  form: UseFormReturn<MemoryFormValues>;
  isPending: boolean;
  matchNotice?: ReactNode;
  onSubmit: (values: MemoryFormValues) => void;
}) {
  const { t } = useTranslation();
  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="grid gap-4">
        <MemoryScopeField form={form} />
        <MemoryNamespaceField control={form.control} />
        <MemoryFactFields control={form.control} />
        <EvidenceFields form={form} />
        {matchNotice}
        <MemoryInputField
          control={form.control}
          name="reason"
          label="memory.reason"
          placeholder="memory.reasonPlaceholder"
        />
        <Button
          type="submit"
          disabled={!form.formState.isValid || isPending}
          className="justify-self-start"
        >
          {t("memory.saveMemory")}
        </Button>
      </form>
    </Form>
  );
}
