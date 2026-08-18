import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Panel } from "@/components/shared/panel";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { LogoMark } from "@/components/shared/logo";
import {
  brandCSSVariables,
  defaultBranding,
  normaliseBranding,
} from "@/features/branding/model";
import {
  useAdminBranding,
  useSetAdminBranding,
  type Branding,
} from "@/features/branding/api";

interface BrandingForm {
  displayName: string;
  logoUrl: string;
  iconUrl: string;
  primaryColor: string;
}

export function BrandingPanel() {
  const { t } = useTranslation();
  const { data, isLoading, error, refetch } = useAdminBranding();
  const save = useSetAdminBranding();
  const [draft, setDraft] = useState<BrandingForm | null>(null);

  if (isLoading) return <LoadingRows rows={4} />;
  if (error) return <ErrorState error={error} onRetry={() => void refetch()} />;

  const stored = formFrom(data);
  const current = draft ?? stored;
  const body = bodyFrom(current);
  const invalidColour = !colourOK(current.primaryColor);
  const cannotSave =
    current.displayName.trim() === "" || invalidColour || save.isPending;

  function change(patch: Partial<BrandingForm>) {
    setDraft({ ...current, ...patch });
  }

  function reset() {
    setDraft(formFrom(defaultBranding));
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
        <Button variant="outline" size="sm" onClick={reset}>
          {t("branding.reset")}
        </Button>
      }
    >
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_320px]">
        <div className="grid gap-4 md:grid-cols-2">
          <BrandField
            id="brand-display"
            label={t("branding.displayName")}
            value={current.displayName}
            onChange={(displayName) => change({ displayName })}
          />
          <BrandField
            id="brand-primary"
            label={t("branding.primaryColor")}
            value={current.primaryColor}
            onChange={(primaryColor) => change({ primaryColor })}
            placeholder={t("branding.primaryPlaceholder")}
            type="text"
            invalid={invalidColour}
            hint={
              invalidColour
                ? t("branding.primaryInvalid")
                : t("branding.primaryHint")
            }
          />
          <BrandField
            id="brand-logo"
            label={t("branding.logoUrl")}
            value={current.logoUrl}
            onChange={(logoUrl) => change({ logoUrl })}
            placeholder={t("branding.logoPlaceholder")}
            hint={t("branding.logoHint")}
          />
          <BrandField
            id="brand-icon"
            label={t("branding.iconUrl")}
            value={current.iconUrl}
            onChange={(iconUrl) => change({ iconUrl })}
            placeholder={t("branding.iconPlaceholder")}
            hint={t("branding.iconHint")}
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

function BrandField({
  id,
  label,
  value,
  onChange,
  placeholder,
  type = "url",
  hint,
  invalid,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  type?: string;
  hint?: string;
  invalid?: boolean;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        type={type}
        value={value}
        placeholder={placeholder}
        autoComplete="off"
        aria-invalid={invalid || undefined}
        className={type === "text" ? "font-mono" : undefined}
        onChange={(e) => onChange(e.target.value)}
      />
      {hint && (
        <p className={invalid ? "text-xs text-destructive" : "text-xs text-muted-foreground"}>
          {hint}
        </p>
      )}
    </div>
  );
}

function BrandPreview({ branding }: { branding: Branding }) {
  const { t } = useTranslation();
  const resolved = normaliseBranding(branding);
  const style = brandCSSVariables(resolved);
  // Custom SVG data images stay in <img>. Inline SVG or CSS backgrounds have different safety rules.
  return (
    <div
      style={style}
      className="flex flex-col gap-3 rounded-lg border border-border bg-card p-4"
    >
      <div className="flex items-center gap-2">
        <span className="grid size-10 shrink-0 place-items-center rounded-lg bg-muted">
          {resolved.iconUrl ? (
            <img
              src={resolved.iconUrl}
              alt={resolved.displayName}
              className="size-7 object-contain"
            />
          ) : (
            <LogoMark size={28} ariaLabel={resolved.displayName} />
          )}
        </span>
        <div className="min-w-0">
          {resolved.logoUrl ? (
            <img
              src={resolved.logoUrl}
              alt={resolved.displayName}
              className="h-7 max-w-[220px] object-contain object-left"
            />
          ) : (
            <p className="truncate text-sm font-medium">
              {resolved.displayName}
            </p>
          )}
          <p className="text-xs text-muted-foreground">
            {t("branding.preview")}
          </p>
        </div>
      </div>
      <div className="flex items-center gap-2">
        <span className="size-6 rounded-md border border-border bg-primary" />
        <Button size="sm">{t("branding.previewButton")}</Button>
      </div>
    </div>
  );
}

function formFrom(branding: Branding | undefined): BrandingForm {
  const normal = normaliseBranding(branding);
  return {
    displayName: normal.displayName,
    logoUrl: normal.logoUrl ?? "",
    iconUrl: normal.iconUrl ?? "",
    primaryColor: normal.primaryColor ?? "",
  };
}

function bodyFrom(form: BrandingForm): Branding {
  return {
    displayName: form.displayName.trim(),
    logoUrl: clean(form.logoUrl),
    iconUrl: clean(form.iconUrl),
    primaryColor: clean(form.primaryColor),
  };
}

function clean(value: string): string | undefined {
  const trimmed = value.trim();
  return trimmed || undefined;
}

function colourOK(value: string): boolean {
  const trimmed = value.trim();
  return trimmed === "" || /^#[0-9A-Fa-f]{6}$/.test(trimmed);
}
