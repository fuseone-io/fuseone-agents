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
import type { ScopeOption } from "@/features/admin/grant-scope-options";

/** One of a grant's two scope selects, with what its options are named. */
export function ScopeSelect({
  field,
  value,
  options,
  disabled,
  onChange,
}: {
  field: { id: string; label: string; labelClass: string };
  value: string;
  options: ScopeOption[];
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <Label htmlFor={field.id} className={field.labelClass}>
        {field.label}
      </Label>
      <Select value={value} onValueChange={onChange} disabled={disabled}>
        <SelectTrigger id={field.id} className="!h-9 w-full">
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
    </div>
  );
}

function optionLabel(option: ScopeOption, t: TFunction) {
  switch (option.state) {
    case "whole":
      return t("people.wholeCompany");
    case "installation":
      return t("people.wholeInstallation");
    case "withdrawn":
      return t("people.withdrawnScope", { label: option.label });
    case "unknown":
      return t("people.unknownScope", { label: option.label });
    default:
      return option.label;
  }
}
