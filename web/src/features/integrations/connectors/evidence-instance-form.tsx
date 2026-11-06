import { zodResolver } from "@hookform/resolvers/zod";
import { useForm, type Resolver } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Form } from "@/components/ui/form";
import {
  PropertiesSheet,
  PropertiesSheetBody,
  PropertiesSheetFooter,
} from "@/components/shared/properties-sheet";
import type {
  ConnectorEvidenceConfig,
  ConnectorInstanceDetail,
} from "@/features/integrations/api";
import { ConnectorIdentityFields } from "@/features/integrations/connectors/connector-identity-fields";
import type { ConnectorInstanceSaver } from "@/features/integrations/connectors/connector-instance-model";
import { EvidenceDeclarationImport } from "@/features/integrations/connectors/evidence-declaration-import";
import {
  evidenceDeclarationValues,
  evidenceInstanceDefaults,
  evidenceInstancePayload,
  evidenceInstanceSchema,
  type EvidenceInstanceValues,
} from "@/features/integrations/connectors/evidence-instance-model";
import { EvidencePagingFields } from "@/features/integrations/connectors/evidence-paging-fields";
import { EvidenceQueryEditor } from "@/features/integrations/connectors/evidence-query-editor";
import { EvidenceSourceFields } from "@/features/integrations/connectors/evidence-source-fields";
import { EvidenceTokenFields } from "@/features/integrations/connectors/evidence-token-fields";
import { useActiveScope } from "@/features/scope/active-scope";
import { problemMessage } from "@/lib/api/problem-message";

export function EvidenceInstanceForm({
  instance,
  onClose,
  onSave,
}: {
  instance: ConnectorInstanceDetail | null;
  onClose: () => void;
  onSave: ConnectorInstanceSaver;
}) {
  const { t } = useTranslation();
  const company = useActiveScope((state) => state.company);
  const area = useActiveScope((state) => state.area);
  const form = useForm<EvidenceInstanceValues>({
    // zod 4 types coerced fields' INPUT as unknown; this form is typed by its
    // OUTPUT, which is also what every field submits.
    resolver: zodResolver(
      evidenceInstanceSchema,
    ) as Resolver<EvidenceInstanceValues>,
    mode: "onChange",
    defaultValues: evidenceInstanceDefaults(instance, company, area),
  });
  const hasToken = instance?.hasToken === true;

  // Replaces the declaration and nothing else: who owns the source and the
  // token a person already typed are not part of an example.
  function applyDeclaration(declaration: ConnectorEvidenceConfig) {
    form.reset({
      ...form.getValues(),
      ...evidenceDeclarationValues(declaration),
    });
  }

  async function submit(values: EvidenceInstanceValues) {
    const input = evidenceInstancePayload(values, hasToken);
    if (!input) {
      form.setError("token", { message: "connectors.tokenRequired" });
      return;
    }
    try {
      await onSave(input);
      toast.success(t("connectors.instanceSaved"));
      onClose();
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  return (
    <PropertiesSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={
        instance ? t("connectors.editInstance") : t("connectors.newEvidence")
      }
      description={t("connectors.evidenceSheetHint")}
      className="lg:max-w-[880px]"
    >
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(submit)}
          className="flex min-h-0 flex-1 flex-col"
        >
          <PropertiesSheetBody className="space-y-6">
            <ConnectorIdentityFields
              editing={instance !== null}
              connector="evidence"
            />
            <EvidenceDeclarationImport onApply={applyDeclaration} />
            <EvidenceSourceFields />
            <EvidencePagingFields />
            <EvidenceQueryEditor />
            <EvidenceTokenFields hasToken={hasToken} />
          </PropertiesSheetBody>
          <PropertiesSheetFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={form.formState.isSubmitting}>
              {t("common.save")}
            </Button>
          </PropertiesSheetFooter>
        </form>
      </Form>
    </PropertiesSheet>
  );
}
