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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

export interface AreaFormValues {
  company: string;
  name: string;
  label: string;
}

export function AreaFormFields({
  control,
  companyOptions,
  editing,
}: {
  control: Control<AreaFormValues>;
  companyOptions: string[];
  editing: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      <FormField
        control={control}
        name="company"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("admin.company")}</FormLabel>
            <Select
              onValueChange={field.onChange}
              value={field.value}
              disabled={editing}
            >
              <FormControl>
                <SelectTrigger>
                  <SelectValue placeholder={t("admin.company")} />
                </SelectTrigger>
              </FormControl>
              <SelectContent>
                {companyOptions.map((company) => (
                  <SelectItem key={company} value={company}>
                    {company}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FormDescription>{t("admin.companyFixed")}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={control}
        name="name"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("admin.name")}</FormLabel>
            <FormControl>
              <Input
                {...field}
                placeholder={t("admin.areaExample")}
                disabled={editing}
              />
            </FormControl>
            <FormDescription>
              {t(editing ? "admin.areaIdFixed" : "admin.areaFolds")}
            </FormDescription>
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
            <FormDescription>{t("admin.emptyUsesName")}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  );
}
