import { zodResolver } from "@hookform/resolvers/zod";
import { useMemo } from "react";
import { useForm, useWatch } from "react-hook-form";
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
  ConnectorInstance,
  ConnectorInstanceDetail,
} from "@/features/integrations/api";
import { ConnectorIdentityFields } from "@/features/integrations/connectors/connector-identity-fields";
import type { ConnectorInstanceSaver } from "@/features/integrations/connectors/connector-instance-model";
import {
  graviteeBindingIssue,
  graviteeInstanceDefaults,
  graviteeInstancePayload,
  graviteeInstanceSchema,
  type GraviteeInstanceValues,
} from "@/features/integrations/connectors/gravitee-instance-model";
import { GraviteePolicyFields } from "@/features/integrations/connectors/gravitee-policy-fields";
import { GraviteeTargetFields } from "@/features/integrations/connectors/gravitee-target-fields";
import { GraviteeVaultFields } from "@/features/integrations/connectors/gravitee-vault-fields";
import { vaultChoices } from "@/features/integrations/connectors/vault-instance-options";
import { useActiveScope } from "@/features/scope/active-scope";
import { problemMessage } from "@/lib/api/problem-message";

export function GraviteeInstanceForm({
  instance,
  instances,
  onClose,
  onSave,
}: {
  instance: ConnectorInstanceDetail | null;
  instances: ConnectorInstance[];
  onClose: () => void;
  onSave: ConnectorInstanceSaver;
}) {
  const { t } = useTranslation();
  const company = useActiveScope((state) => state.company);
  const area = useActiveScope((state) => state.area);
  const form = useForm<GraviteeInstanceValues>({
    resolver: zodResolver(graviteeInstanceSchema, undefined, { mode: "sync" }),
    mode: "onChange",
    defaultValues: graviteeInstanceDefaults(instance, company, area),
  });
  const [scopeKind, targetCompany, targetArea] = useWatch({
    control: form.control,
    name: ["scopeKind", "company", "area"],
  });
  const choices = useMemo(
    () => vaultChoices(instances, {
      scopeKind,
      company: targetCompany,
      area: targetArea,
    }),
    [instances, scopeKind, targetCompany, targetArea],
  );

  async function submit(values: GraviteeInstanceValues) {
    form.clearErrors(["vaultInstance", "credentialPath"]);
    const issue = values.enabled
      ? graviteeBindingIssue(instances, values, values.vaultInstance, values.credentialPath)
      : null;
    if (issue) {
      form.setError(
        issue === "connectors.graviteeVaultPathOutside" ? "credentialPath" : "vaultInstance",
        { message: issue },
      );
      return;
    }
    try {
      await onSave(graviteeInstancePayload(values));
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
      title={instance ? t("connectors.editGraviteeInstance") : t("connectors.newGravitee")}
      description={t("connectors.graviteeInstanceSheetHint")}
      className="lg:max-w-[760px]"
    >
      <Form {...form}>
        <form onSubmit={form.handleSubmit(submit)} className="flex min-h-0 flex-1 flex-col">
          <PropertiesSheetBody className="space-y-6">
            <ConnectorIdentityFields editing={instance !== null} connector="gravitee" />
            <GraviteeTargetFields form={form} />
            <GraviteePolicyFields form={form} />
            <GraviteeVaultFields form={form} choices={choices} />
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
