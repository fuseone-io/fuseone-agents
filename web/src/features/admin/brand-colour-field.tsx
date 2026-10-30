import { useTranslation } from "react-i18next";
import { ColorInput } from "@/components/ui/color-input";
import { BrandField } from "@/features/admin/brand-field";
import { colourOK, pickerValue } from "@/features/admin/branding-form";

/**
 * The primary colour, typed as hex or picked visually. Both controls write
 * the same value, so neither can drift from what will be saved; the server
 * still validates #RRGGBB on its own.
 */
export function BrandColourField({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const invalid = !colourOK(value);
  return (
    <BrandField
      field={{
        id: "brand-primary",
        label: t("branding.primaryColor"),
        placeholder: t("branding.primaryPlaceholder"),
        type: "text",
        mono: true,
        invalid,
        hint: invalid ? t("branding.primaryInvalid") : t("branding.primaryHint"),
      }}
      value={value}
      onChange={onChange}
    >
      <ColorInput
        aria-label={t("branding.primaryPicker")}
        value={pickerValue(value)}
        onChange={(e) => onChange(e.target.value)}
      />
    </BrandField>
  );
}
