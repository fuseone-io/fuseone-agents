import { useTranslation } from "react-i18next";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import type { RulingLevel } from "@/features/admin/api";
import type { RulingOffer } from "@/features/admin/classify-offer";

const LEVELS: RulingLevel[] = ["platform", "owner"];

/**
 * Whose ruling is being recorded (NT-019).
 *
 * Asked only of the platform's curator on a company's or an area's server,
 * and starting on the ceiling, which is what that curator always recorded.
 * An area's curator records the owner's and is told so rather than asked.
 */
export function RulingLevelField({
  offer,
  level,
  onChange,
}: {
  offer: RulingOffer;
  level: RulingLevel;
  onChange: (level: RulingLevel) => void;
}) {
  const { t } = useTranslation();
  if (offer === "platform") return null;
  if (offer === "owner") {
    return (
      <p className="text-xs text-muted-foreground">{t("admin.recordedAsOwner")}</p>
    );
  }
  return (
    <fieldset className="space-y-2">
      <legend className="text-sm font-medium">{t("admin.rulingLevelLabel")}</legend>
      <RadioGroup
        value={level}
        onValueChange={(next) => onChange(next as RulingLevel)}
      >
        {LEVELS.map((value) => (
          <div key={value} className="flex items-start gap-2">
            <RadioGroupItem id={`ruling-level-${value}`} value={value} className="mt-0.5" />
            <Label htmlFor={`ruling-level-${value}`} className="flex-col items-start gap-0.5">
              <span>{t(`admin.rulingLevel.${value}`)}</span>
              <span className="text-xs font-normal text-muted-foreground">
                {t(`admin.rulingLevelHint.${value}`)}
              </span>
            </Label>
          </div>
        ))}
      </RadioGroup>
    </fieldset>
  );
}
