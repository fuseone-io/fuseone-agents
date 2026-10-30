import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { LogoMark } from "@/components/shared/logo";
import type { Branding } from "@/features/branding/api";
import {
  brandCSSVariables,
  normaliseBranding,
} from "@/features/branding/model";

export function BrandPreview({ branding }: { branding: Branding }) {
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
