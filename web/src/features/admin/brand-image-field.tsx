import { useState, type ChangeEvent } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { BrandField, type BrandFieldConfig } from "@/features/admin/brand-field";
import { useUploadBrandAsset } from "@/features/branding/api";
import { maxBrandAssetBytes } from "@/features/branding/model";

const accepted = "image/png,image/jpeg,image/webp";

/**
 * A logo or icon: an address typed in, or a file uploaded to this
 * installation, whose internal address then fills the same field. Either way
 * the brand stores an address, never the image.
 */
export function BrandImageField({
  field,
  uploadLabel,
  value,
  onChange,
}: {
  field: BrandFieldConfig;
  uploadLabel: string;
  value: string;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const upload = useUploadBrandAsset();
  const [refusal, setRefusal] = useState<string | null>(null);
  const uploadId = `${field.id}-upload`;

  function choose(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;
    if (file.size > maxBrandAssetBytes) {
      setRefusal(t("branding.uploadTooLarge"));
      return;
    }
    setRefusal(null);
    upload.mutate(file, {
      onSuccess: (asset) => onChange(asset.url),
      onError: (err) =>
        setRefusal(err instanceof Error ? err.message : t("branding.uploadFailed")),
    });
  }

  const footer = (
    <div className="flex flex-col gap-1">
      <Label htmlFor={uploadId} className="text-xs text-muted-foreground">
        {uploadLabel}
      </Label>
      <Input
        id={uploadId}
        type="file"
        accept={accepted}
        disabled={upload.isPending}
        onChange={choose}
      />
      <p className={refusal ? "text-xs text-destructive" : "text-xs text-muted-foreground"}>
        {refusal ?? t("branding.uploadHint")}
      </p>
    </div>
  );

  return <BrandField field={field} value={value} onChange={onChange} footer={footer} />;
}
