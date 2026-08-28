import { zodResolver } from "@hookform/resolvers/zod";
import { Trash2, TriangleAlert, X } from "lucide-react";
import { useEffect } from "react";
import { useForm, useWatch } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Panel } from "@/components/shared/panel";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
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
  visible = true,
  onExit,
  onDone,
  onDiscard,
  onDirtyChange,
}: {
  framed?: boolean;
  visible?: boolean;
  onExit?: () => void;
  onDone?: () => void;
  onDiscard?: () => void;
  onDirtyChange?: (dirty: boolean) => void;
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
  const match = useMemoryCreateMatch(form, { enabled: visible });
  const reason = useWatch({ control: form.control, name: "reason" });
  const company = useWatch({ control: form.control, name: "company" });
  const area = useWatch({ control: form.control, name: "area" });
  const isDirty = form.formState.isDirty;

  useEffect(() => {
    const scope = memoryDefaults(activeCompany, activeArea);
    form.resetField("evidenceRunId", { defaultValue: "" });
    form.resetField("company", { defaultValue: scope.company });
    form.resetField("area", { defaultValue: scope.area });
  }, [activeArea, activeCompany, form]);

  useEffect(() => {
    onDirtyChange?.(isDirty);
  }, [isDirty, onDirtyChange]);

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

  function discardDraft() {
    form.reset(memoryDefaults(activeCompany, activeArea));
    (onDiscard ?? onExit)?.();
  }

  const content = (
    <>
      <p className="mb-4 text-sm text-muted-foreground">
        {t("memory.createHint")}
      </p>
      {(!company || !area) && (
        <p
          role="status"
          className="mb-4 flex items-start gap-2 rounded-md bg-warning-surface px-3 py-2 text-xs text-warning"
        >
          <TriangleAlert className="mt-0.5 size-3.5 shrink-0" aria-hidden />
          <span>{t("memory.companyRequiredForCreation")}</span>
        </p>
      )}
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
        <header className="mb-4 flex items-center justify-between gap-3">
          <h2 className="text-base font-medium">{t("memory.newAssertion")}</h2>
          <div className="flex items-center gap-2">
            {isDirty && (
              <AlertDialog>
                <AlertDialogTrigger asChild>
                  <Button type="button" variant="ghost" size="sm">
                    <Trash2 aria-hidden />
                    {t("memory.discardDraft")}
                  </Button>
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>
                      {t("memory.discardDraftTitle")}
                    </AlertDialogTitle>
                    <AlertDialogDescription>
                      {t("memory.discardDraftHint")}
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
                    <AlertDialogAction
                      variant="destructive"
                      onClick={discardDraft}
                    >
                      {t("memory.discardDraft")}
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            )}
            {onExit && (
              <Button type="button" variant="outline" size="sm" onClick={onExit}>
                <X aria-hidden />
                {t("common.close")}
              </Button>
            )}
          </div>
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
