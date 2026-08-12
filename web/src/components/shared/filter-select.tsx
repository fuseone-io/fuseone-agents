import { useTranslation } from "react-i18next";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

export interface FilterOption {
  value: string;
  label: string;
}

/**
 * A filter dropdown at the console's row height.
 *
 * shadcn's trigger is 36px, which is right for a form and one step too tall
 * beside a 32px search field — the design keeps a dense screen on one rhythm.
 */
export function FilterSelect({
  label,
  value,
  options,
  onChange,
  width = 200,
}: {
  label: string;
  value: string;
  options: FilterOption[];
  onChange: (value: string) => void;
  width?: number;
}) {
  const { t } = useTranslation();
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger
        aria-label={t(label)}
        style={{ width }}
        className="h-8 shrink-0 gap-2 rounded-sm px-2.5 text-sm font-medium data-[size=default]:h-8"
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option.value} value={option.value}>
            {t(option.label)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
