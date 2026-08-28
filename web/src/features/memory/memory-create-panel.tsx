import { zodResolver } from "@hookform/resolvers/zod";
import { useForm, useWatch } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Panel } from "@/components/shared/panel";
import { useActiveScope } from "@/features/scope/active-scope";
import { useCreateMemoryAssertion } from "@/features/memory/api";
import {
  memoryDefaults,
  toMemoryAssertionInput,
} from "@/features/memory/memory-create-model";
import { MemoryCreateForm } from "@/features/memory/memory-create-form";
import { useMemoryCreateMatch } from "@/features/memory/memory-create-match";
import { MemoryCreateMatchNotice } from "@/features/memory/memory-create-match-notice";
import {
  memoryFormSchema,
  type MemoryFormValues,
} from "@/features/memory/memory-form-schema";
import { problemMessage } from "@/lib/api/problem-message";

export function MemoryCreatePanel({
  framed = true,
  onDone,
}: {
  framed?: boolean;
  onDone?: () => void;
}) {
  const { t } = useTranslation();
  const create = useCreateMemoryAssertion();
  const activeCompany = useActiveScope((s) => s.company);
  const activeArea = useActiveScope((s) => s.area);
  const form = useForm<MemoryFormValues>({
    resolver: zodResolver(memoryFormSchema),
    mode: "onChange",
    defaultValues: memoryDefaults(activeCompany, activeArea),
  });
  const match = useMemoryCreateMatch(form);
  const reason = useWatch({ control: form.control, name: "reason" });

  async function submit(values: MemoryFormValues) {
    try {
      await create.mutateAsync(toMemoryAssertionInput(values));
      toast.success(t("memory.recorded"));
      form.reset(memoryDefaults(values.company.trim(), values.area.trim()));
      onDone?.();
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  const content = (
    <>
      <p className="mb-4 text-sm text-muted-foreground">
        {t("memory.createHint")}
      </p>
      <MemoryCreateForm
        form={form}
        isPending={create.isPending || (match.required && !match.ready)}
        matchNotice={
          <MemoryCreateMatchNotice
            state={match}
            reason={reason}
            onImproveShared={() =>
              form.setValue("namespace", "shared", {
                shouldDirty: true,
                shouldValidate: true,
              })
            }
          />
        }
        onSubmit={submit}
      />
    </>
  );

  if (!framed) {
    return (
      <section className="mx-auto w-full max-w-[820px] min-w-0">
        <header className="mb-4">
          <h2 className="text-base font-medium">{t("memory.newAssertion")}</h2>
        </header>
        {content}
      </section>
    );
  }

  return (
    <Panel title={t("memory.newAssertion")}>
      {content}
    </Panel>
  );
}
