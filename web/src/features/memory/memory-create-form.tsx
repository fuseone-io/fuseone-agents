import type { UseFormReturn } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Form } from "@/components/ui/form";
import { EvidenceFields } from "@/features/memory/memory-evidence-fields";
import {
  MemoryInputField,
  MemoryTextareaField,
} from "@/features/memory/memory-form-fields";
import { MemoryNamespaceField } from "@/features/memory/memory-namespace-field";
import type { MemoryFormValues } from "@/features/memory/memory-form-schema";

export function MemoryCreateForm({
  form,
  isPending,
  onSubmit,
}: {
  form: UseFormReturn<MemoryFormValues>;
  isPending: boolean;
  onSubmit: (values: MemoryFormValues) => void;
}) {
  const { t } = useTranslation();
  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="grid gap-4">
        <div className="grid gap-3 sm:grid-cols-2">
          <MemoryInputField
            control={form.control}
            name="company"
            label="admin.company"
            className="font-mono"
          />
          <MemoryInputField
            control={form.control}
            name="area"
            label="admin.area"
            className="font-mono"
          />
        </div>
        <MemoryNamespaceField control={form.control} />
        <MemoryInputField
          control={form.control}
          name="kind"
          label="memory.kind"
          placeholder="memory.kindPlaceholder"
        />
        <MemoryInputField
          control={form.control}
          name="subject"
          label="memory.subject"
          placeholder="memory.subjectPlaceholder"
        />
        <MemoryInputField
          control={form.control}
          name="signature"
          label="memory.signature"
          description="memory.signatureHint"
          className="font-mono"
        />
        <MemoryTextareaField
          control={form.control}
          name="claim"
          label="memory.claim"
          description="memory.claimHint"
        />
        <EvidenceFields control={form.control} />
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
