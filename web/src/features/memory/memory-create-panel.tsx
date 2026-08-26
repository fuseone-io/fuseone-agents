import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Form } from "@/components/ui/form";
import { Panel } from "@/components/shared/panel";
import { useActiveScope } from "@/features/scope/active-scope";
import { useCreateMemoryAssertion } from "@/features/memory/api";
import {
  memoryDefaults,
  toMemoryAssertionInput,
} from "@/features/memory/memory-create-model";
import { EvidenceFields } from "@/features/memory/memory-evidence-fields";
import {
  MemoryInputField,
  MemoryTextareaField,
} from "@/features/memory/memory-form-fields";
import {
  memoryFormSchema,
  type MemoryFormValues,
} from "@/features/memory/memory-form-schema";
import { problemMessage } from "@/lib/api/problem-message";

export function MemoryCreatePanel() {
  const { t } = useTranslation();
  const create = useCreateMemoryAssertion();
  const activeCompany = useActiveScope((s) => s.company);
  const activeArea = useActiveScope((s) => s.area);
  const form = useForm<MemoryFormValues>({
    resolver: zodResolver(memoryFormSchema),
    mode: "onChange",
    defaultValues: memoryDefaults(activeCompany, activeArea),
  });

  async function submit(values: MemoryFormValues) {
    try {
      await create.mutateAsync(toMemoryAssertionInput(values));
      toast.success(t("memory.recorded"));
      form.reset(memoryDefaults(values.company.trim(), values.area.trim()));
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  return (
    <Panel title={t("memory.newAssertion")}>
      <p className="mb-4 text-sm text-muted-foreground">
        {t("memory.createHint")}
      </p>
      <Form {...form}>
        <form onSubmit={form.handleSubmit(submit)} className="grid gap-4">
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
          <MemoryInputField
            control={form.control}
            name="agentId"
            label="memory.agentOptional"
            description="memory.agentHint"
            className="font-mono"
          />
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
          <div className="grid gap-3 sm:grid-cols-2">
            <MemoryInputField
              control={form.control}
              name="observations"
              label="memory.observations"
              className="font-mono"
            />
            <MemoryInputField
              control={form.control}
              name="confirmed"
              label="memory.confirmed"
              className="font-mono"
            />
          </div>
          <EvidenceFields control={form.control} />
          <MemoryInputField
            control={form.control}
            name="reason"
            label="memory.reason"
            placeholder="memory.reasonPlaceholder"
          />
          <Button
            type="submit"
            disabled={!form.formState.isValid || create.isPending}
          >
            {t("memory.record")}
          </Button>
        </form>
      </Form>
    </Panel>
  );
}
