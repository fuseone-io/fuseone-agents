import { useFormContext } from "react-hook-form";
import { useTranslation } from "react-i18next";
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { ScopeValues } from "@/features/integrations/scope/scope-choice";

/** Any scope, named freely: for the platform's administration. */
export function FreeScopeFields({ editing, label }: { editing: boolean; label: string }) {
  const { t } = useTranslation();
  const form = useFormContext<ScopeValues>();
  const scopeKind = form.watch("scopeKind");
  return (
    <>
      <FormField
        control={form.control}
        name="scopeKind"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t(label)}</FormLabel>
            <Select
              value={field.value}
              onValueChange={field.onChange}
              disabled={editing}
            >
              <FormControl>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
              </FormControl>
              <SelectContent>
                <SelectItem value="area">
                  {t("connectors.scope.area")}
                </SelectItem>
                <SelectItem value="company">
                  {t("connectors.scope.company")}
                </SelectItem>
                <SelectItem value="installation">
                  {t("connectors.scope.installation")}
                </SelectItem>
              </SelectContent>
            </Select>
            <FormMessage />
          </FormItem>
        )}
      />
      {scopeKind !== "installation" && (
        <div className="grid gap-3 sm:grid-cols-2">
          <IdentityTextField
            name="company"
            label={t("admin.company")}
            disabled={editing}
          />
          {scopeKind === "area" && (
            <IdentityTextField
              name="area"
              label={t("admin.area")}
              disabled={editing}
            />
          )}
        </div>
      )}
    </>
  );
}

function IdentityTextField({
  name,
  label,
  disabled,
}: {
  name: "company" | "area";
  label: string;
  disabled: boolean;
}) {
  const form = useFormContext<ScopeValues>();
  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <Input
              {...field}
              disabled={disabled}
              autoComplete="off"
              className="font-mono"
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
