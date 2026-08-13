import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Separator } from "@/components/ui/separator";
import {
  IdentityFields,
  type IdentityForm,
} from "@/features/admin/identity-fields";
import { MappingBuilder } from "@/features/admin/mapping-builder";
import {
  usePutIdentityProvider,
  type GroupMapping,
  type IdentityProvider,
} from "@/features/admin/identity-api";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * One way of signing in.
 *
 * The secret field is empty on an edit and keeps whatever is stored: fixing a
 * group mapping must not demand a credential nobody has to hand, or the
 * mapping stays wrong.
 */
export function IdentityProviderForm({
  provider,
  onDone,
}: {
  provider?: IdentityProvider;
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const save = usePutIdentityProvider();
  const [form, setForm] = useState<IdentityForm>({
    id: provider?.id ?? "",
    display: provider?.display ?? "",
    issuer: provider?.issuer ?? "",
    clientId: provider?.clientId ?? "",
    clientSecret: "",
    groupsClaim: provider?.groupsClaim ?? "",
    enabled: provider?.enabled ?? true,
  });
  const [mappings, setMappings] = useState<GroupMapping[]>(
    provider?.mappings ?? [],
  );

  const submit = () =>
    save.mutate(
      {
        id: form.id,
        body: {
          display: form.display,
          issuer: form.issuer,
          clientId: form.clientId,
          clientSecret: form.clientSecret || undefined,
          groupsClaim: form.groupsClaim || undefined,
          mappings,
          enabled: form.enabled,
        },
      },
      {
        onSuccess: () => {
          toast.success(t("identity.saved", { name: form.id }));
          onDone();
        },
        // The server discovers the issuer as it saves, so this is where a
        // wrong address is reported — with what actually failed.
        onError: (error) =>
          toast.error(t("identity.saveFailed"), {
            description: problemMessage(error, t),
          }),
      },
    );

  return (
    <div className="flex flex-col gap-4">
      <IdentityFields
        form={form}
        onChange={setForm}
        hasSecret={provider?.hasSecret === true}
        editing={provider !== undefined}
      />

      <div className="flex items-center gap-2">
        <Switch
          id="idp-enabled"
          checked={form.enabled}
          onCheckedChange={(enabled) => setForm({ ...form, enabled })}
        />
        <Label htmlFor="idp-enabled">{t("identity.enabled")}</Label>
      </div>

      <Separator />

      <div className="flex flex-col gap-2">
        <h3 className="text-sm font-medium">{t("identity.mappings")}</h3>
        <p className="text-xs text-muted-foreground">
          {t("identity.mappingsHelp")}
        </p>
        <MappingBuilder mappings={mappings} onChange={setMappings} />
      </div>

      <div className="flex gap-2">
        <Button onClick={submit} disabled={save.isPending || form.id === ""}>
          {t("common.save")}
        </Button>
        <Button variant="ghost" onClick={onDone}>
          {t("common.cancel")}
        </Button>
      </div>
    </div>
  );
}
