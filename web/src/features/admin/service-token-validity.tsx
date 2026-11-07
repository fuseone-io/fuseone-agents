import type { Control } from "react-hook-form";
import { useTranslation } from "react-i18next";
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

const VALIDITY_DAYS = [30, 90, 180, 365];

export function ServiceTokenValidity({
  control,
}: {
  control: Control<{ name: string; days: string }>;
}) {
  const { t } = useTranslation();
  return (
    <FormField
      control={control}
      name="days"
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t("serviceAccounts.validity")}</FormLabel>
          <Select value={field.value} onValueChange={field.onChange}>
            <FormControl>
              <SelectTrigger className="w-full">
                <SelectValue placeholder={t("serviceAccounts.validityPlaceholder")} />
              </SelectTrigger>
            </FormControl>
            <SelectContent>
              {VALIDITY_DAYS.map((days) => (
                <SelectItem key={days} value={String(days)}>
                  {t("serviceAccounts.validityDays", { count: days })}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <FormDescription>{t("serviceAccounts.validityHint")}</FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
