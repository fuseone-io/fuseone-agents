import { Search } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";

export function ManualSearchBox({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <label className="relative block min-w-0">
      <span className="sr-only">{t("manual.search")}</span>
      <Search className="pointer-events-none absolute top-2.5 left-3 size-4 text-muted-foreground" />
      <Input
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={t("manual.searchPlaceholder")}
        className="pl-9"
      />
    </label>
  );
}
