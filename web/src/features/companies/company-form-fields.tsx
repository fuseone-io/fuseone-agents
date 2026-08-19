import { useTranslation } from "react-i18next";
import type { Control } from "react-hook-form";
import { Input } from "@/components/ui/input";
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";

export interface CompanyFormValues {
  id: string;
  label: string;
}

export function CompanyFormFields({
  control,
  editing,
}: {
  control: Control<CompanyFormValues>;
  editing: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      <FormField
        control={control}
        name="id"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("companies.identifier")}</FormLabel>
            <FormControl>
              <Input
                {...field}
                className="font-mono"
                placeholder="acme"
                disabled={editing}
              />
            </FormControl>
            <FormDescription>{t("companies.idNeverChanges")}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={control}
        name="label"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("admin.shownAs")}</FormLabel>
            <FormControl>
              <Input {...field} placeholder={t("common.optional")} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  );
}
