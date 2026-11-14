import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { ScopeOption } from "@/features/scope/scope-select-options";

export type ScopeSelectField = {
  id: string;
  label: string;
  labelClass?: string;
  /** An i18n key, shown under the select when the form refuses its value. */
  error?: string;
};

/** A company or an area select, with what its options are named. */
export function ScopeSelect({
  field,
  value,
  options,
  disabled,
  onChange,
}: {
  field: ScopeSelectField;
  value: string;
  options: ScopeOption[];
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const errorId = `${field.id}-error`;
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <Label htmlFor={field.id} className={field.labelClass}>
        {field.label}
      </Label>
      <Select value={value} onValueChange={onChange} disabled={disabled}>
        <SelectTrigger
          id={field.id}
          className="!h-9 w-full"
          aria-invalid={field.error ? true : undefined}
          aria-describedby={field.error ? errorId : undefined}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {optionLabel(option, t)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {field.error && (
        <p id={errorId} className="text-sm text-destructive">
          {t(field.error)}
        </p>
      )}
    </div>
  );
}

function optionLabel(option: ScopeOption, t: TFunction) {
  switch (option.state) {
    case "whole":
      return t("scope.optionWholeCompany");
    case "installation":
      return t("scope.optionWholeInstallation");
    case "withdrawn":
      return t("scope.optionWithdrawn", { label: option.label });
    case "unknown":
      return t("scope.optionUnknown", { label: option.label });
    default:
      return option.label;
  }
}
