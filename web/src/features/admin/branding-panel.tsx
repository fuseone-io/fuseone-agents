import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Panel } from "@/components/shared/panel";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import { Button } from "@/components/ui/button";
import { BrandColourField } from "@/features/admin/brand-colour-field";
import { BrandField } from "@/features/admin/brand-field";
import { BrandImageField } from "@/features/admin/brand-image-field";
import { BrandPreview } from "@/features/admin/brand-preview";
import {
  bodyFrom,
  colourOK,
  formFrom,
  type BrandingForm,
} from "@/features/admin/branding-form";
import { defaultBranding } from "@/features/branding/model";
import {
  useAdminBranding,
  useSetAdminBranding,
} from "@/features/branding/api";

export function BrandingPanel() {
  const { t } = useTranslation();
  const { data, isLoading, error, refetch } = useAdminBranding();
  const save = useSetAdminBranding();
  const [draft, setDraft] = useState<BrandingForm | null>(null);

  if (isLoading) return <LoadingRows rows={4} />;
  if (error) return <ErrorState error={error} onRetry={() => void refetch()} />;

  const current = draft ?? formFrom(data);
  const body = bodyFrom(current);
  const cannotSave =
    current.displayName.trim() === "" ||
    !colourOK(current.primaryColor) ||
    save.isPending;

  function change(patch: Partial<BrandingForm>) {
    setDraft({ ...current, ...patch });
  }

  function submit() {
    save.mutate(body, {
      onSuccess: () => {
        toast.success(t("branding.saved"));
        setDraft(null);
      },
      onError: (e) =>
        toast.error(t("branding.saveFailed"), {
          description: e instanceof Error ? e.message : undefined,
        }),
    });
  }

  return (
    <Panel
      title={t("branding.title")}
      action={
        <Button
          variant="outline"
          size="sm"
          onClick={() => setDraft(formFrom(defaultBranding))}
        >
          {t("branding.reset")}
        </Button>
      }
    >
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_320px]">
        <div className="grid gap-4 md:grid-cols-2">
          <BrandField
            field={{
              id: "brand-display",
              label: t("branding.displayName"),
              type: "text",
            }}
            value={current.displayName}
            onChange={(displayName) => change({ displayName })}
          />
          <BrandColourField
            value={current.primaryColor}
            onChange={(primaryColor) => change({ primaryColor })}
          />
          <BrandImageField
            field={{
              id: "brand-logo",
              label: t("branding.logoUrl"),
              placeholder: t("branding.logoPlaceholder"),
              hint: t("branding.logoHint"),
            }}
            uploadLabel={t("branding.uploadLogo")}
            value={current.logoUrl}
            onChange={(logoUrl) => change({ logoUrl })}
          />
          <BrandImageField
            field={{
              id: "brand-icon",
              label: t("branding.iconUrl"),
              placeholder: t("branding.iconPlaceholder"),
              hint: t("branding.iconHint"),
            }}
            uploadLabel={t("branding.uploadIcon")}
            value={current.iconUrl}
            onChange={(iconUrl) => change({ iconUrl })}
          />
          <p className="rounded-lg border border-warning/30 bg-warning-surface px-3 py-2 text-xs text-warning md:col-span-2">
            {t("branding.externalUrlWarning")}
          </p>
        </div>

        <BrandPreview branding={body} />
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border pt-4">
        <p className="max-w-2xl text-xs text-muted-foreground">
          {t("branding.auditHint")}
        </p>
        <Button onClick={submit} disabled={cannotSave}>
          {t("common.save")}
        </Button>
      </div>
    </Panel>
  );
}
