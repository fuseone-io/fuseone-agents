import { useTranslation } from "react-i18next";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import type { AuthChoice } from "@/features/integrations/mcp/auth-choice";

/**
 * Which shape the credential takes, asked before its fields are shown.
 *
 * Nothing to ask when there is one choice: the fields of that one are the
 * whole form.
 */
export function AuthChoicePicker({
  id,
  choices,
  value,
  onChange,
}: {
  id: string;
  choices: AuthChoice[];
  value: AuthChoice | null;
  onChange: (choice: AuthChoice) => void;
}) {
  const { t } = useTranslation();
  if (choices.length < 2) return null;
  return (
    <fieldset className="space-y-2">
      <legend className="text-sm font-medium">{t("mcp.authChoiceLegend")}</legend>
      <RadioGroup
        value={value ?? undefined}
        onValueChange={(next) => onChange(next as AuthChoice)}
        className="sm:grid-cols-2"
      >
        {choices.map((choice) => (
          <div key={choice} className="flex items-start gap-2 rounded-lg border p-3">
            <RadioGroupItem id={`${id}-${choice}`} value={choice} className="mt-0.5" />
            <Label htmlFor={`${id}-${choice}`} className="flex-col items-start gap-0.5">
              <span>{t(`mcp.authChoice.${choice}`)}</span>
              <span className="text-xs font-normal text-muted-foreground">
                {t(`mcp.authChoiceHint.${choice}`)}
              </span>
            </Label>
          </div>
        ))}
      </RadioGroup>
    </fieldset>
  );
}
