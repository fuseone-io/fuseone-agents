import { useEffect } from "react";
import { useFormContext } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { FormControl, FormItem, FormLabel } from "@/components/ui/form";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { ConnectorIdentityValues } from "@/features/integrations/connectors/connector-identity-fields";
import type { MeScope } from "@/features/session/api";

/** One of the scopes where the caller configures, the first by default. */
const keyOf = (s: MeScope) => `${s.company}/${s.area}`;

export function ConnectorHeldScopeField({ editing, scopes }: { editing: boolean; scopes: MeScope[] }) {
  const { t } = useTranslation();
  const form = useFormContext<ConnectorIdentityValues>();
  const choose = (scope: MeScope) => {
    form.setValue("scopeKind", scope.area ? "area" : "company");
    form.setValue("company", scope.company);
    form.setValue("area", scope.area);
  };
  const company = form.watch("company");
  const area = form.watch("area");
  useEffect(() => {
    const first = scopes[0];
    if (!editing && !company && first) choose(first);
    // Once, for a new instance: a choice the person made is not undone.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return (
    <FormItem>
      <FormLabel>{t("connectors.scopeHeld")}</FormLabel>
      <Select
        value={keyOf({ company, area })}
        onValueChange={(key) => {
          const scope = scopes.find((s) => keyOf(s) === key);
          if (scope) choose(scope);
        }}
        disabled={editing}
      >
        <FormControl>
          <SelectTrigger className="font-mono">
            <SelectValue />
          </SelectTrigger>
        </FormControl>
        <SelectContent>
          {scopes.map((scope) => (
            <SelectItem key={keyOf(scope)} value={keyOf(scope)} className="font-mono">
              {scope.area ? keyOf(scope) : scope.company}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </FormItem>
  );
}

