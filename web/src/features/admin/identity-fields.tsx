import { useTranslation } from "react-i18next";
import { LabelledField } from "@/features/admin/labelled-field";

/** What an identity provider is, minus what it grants. */
export interface IdentityForm {
  id: string;
  display: string;
  issuer: string;
  clientId: string;
  clientSecret: string;
  groupsClaim: string;
  enabled: boolean;
}

export function IdentityFields({
  form,
  onChange,
  hasSecret,
  editing,
}: {
  form: IdentityForm;
  onChange: (form: IdentityForm) => void;
  hasSecret: boolean;
  /** Whether this provider already exists. Its id is what every stored
   *  mapping and the callback URL are keyed on, so editing one cannot rename
   *  it — that would be configuring a second provider. */
  editing: boolean;
}) {
  const { t } = useTranslation();
  const set = (patch: Partial<IdentityForm>) => onChange({ ...form, ...patch });

  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <LabelledField
        field={{
          id: "idp-id",
          label: t("identity.id"),
          value: form.id,
          // The id is what every stored mapping and every callback URL is
          // keyed on. Renaming it would be configuring a second provider.
          disabled: editing,
        }}
        onChange={(id) => set({ id })}
      />
      <LabelledField
        field={{
          id: "idp-display",
          label: t("identity.display"),
          value: form.display,
        }}
        onChange={(display) => set({ display })}
      />
      <LabelledField
        field={{
          id: "idp-issuer",
          label: t("identity.issuer"),
          value: form.issuer,
        }}
        onChange={(issuer) => set({ issuer })}
      />
      <LabelledField
        field={{
          id: "idp-client",
          label: t("identity.clientId"),
          value: form.clientId,
        }}
        onChange={(clientId) => set({ clientId })}
      />
      <LabelledField
        field={{
          id: "idp-secret",
          label: t("identity.clientSecret"),
          value: form.clientSecret,
          type: "password",
          hint: hasSecret ? t("identity.secretKept") : undefined,
        }}
        onChange={(clientSecret) => set({ clientSecret })}
      />
      <LabelledField
        field={{
          id: "idp-claim",
          label: t("identity.groupsClaim"),
          value: form.groupsClaim,
          hint: t("identity.groupsClaimHint"),
        }}
        onChange={(groupsClaim) => set({ groupsClaim })}
      />
    </div>
  );
}
