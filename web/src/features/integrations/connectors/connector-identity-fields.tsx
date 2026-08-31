import { useFormContext } from "react-hook-form";
import { useTranslation } from "react-i18next";
import {
  FormControl,
  FormDescription,
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
import { Switch } from "@/components/ui/switch";

export type ConnectorIdentityValues = {
  name: string;
  enabled: boolean;
  scopeKind: "installation" | "company" | "area";
  company: string;
  area: string;
};

export function ConnectorIdentityFields({
  editing,
  connector,
}: {
  editing: boolean;
  connector: "vault" | "sql";
}) {
  const { t } = useTranslation();
  const form = useFormContext<ConnectorIdentityValues>();
  const scopeKind = form.watch("scopeKind");
  return (
    <div className="grid gap-4">
      <FormField
        control={form.control}
        name="name"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("connectors.instanceName")}</FormLabel>
            <FormControl>
              <Input {...field} disabled={editing} className="font-mono" />
            </FormControl>
            <FormDescription>
              {t("connectors.instanceNameHint", { connector })}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="scopeKind"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("connectors.scopeKind")}</FormLabel>
            <Select
              value={field.value}
              onValueChange={field.onChange}
              disabled={editing}
            >
              <FormControl>
                <SelectTrigger><SelectValue /></SelectTrigger>
              </FormControl>
              <SelectContent>
                <SelectItem value="area">{t("connectors.scope.area")}</SelectItem>
                <SelectItem value="company">{t("connectors.scope.company")}</SelectItem>
                <SelectItem value="installation">{t("connectors.scope.installation")}</SelectItem>
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
      <FormField
        control={form.control}
        name="enabled"
        render={({ field }) => (
          <FormItem className="flex items-center justify-between rounded-lg border p-3">
            <FormLabel className="m-0">{t("integrations.enabled")}</FormLabel>
            <FormControl>
              <Switch checked={field.value} onCheckedChange={field.onChange} />
            </FormControl>
          </FormItem>
        )}
      />
    </div>
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
  const form = useFormContext<ConnectorIdentityValues>();
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
