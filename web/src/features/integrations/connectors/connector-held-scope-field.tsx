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

// Whether a grant at `held` reaches `scope`, as the server's Scope.Contains
// does: a company reaches its areas, an area only itself (NT-019).
const contains = (held: MeScope, scope: MeScope) =>
  held.company === scope.company && (held.area === "" || held.area === scope.area);

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
    // A new form may open on a scope from elsewhere, such as the area the
    // caller works in under another grant: kept, it would be refused on save.
    const held = scopes.some((s) => contains(s, { company, area }));
    if (!editing && !held && first) choose(first);
    // Once, for a new instance: a choice the person made is not undone.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  // The scope in the form is offered too when a grant covers it without
  // naming it, such as an area of a held company. That covers an instance
  // being edited: only an instance the caller may change opens, and that means
  // a held scope contains it.
  const current = { company, area };
  const listed = scopes.some((s) => keyOf(s) === keyOf(current));
  const shown =
    company && !listed && scopes.some((s) => contains(s, current))
      ? [current, ...scopes]
      : scopes;
  return (
    <FormItem>
      <FormLabel>{t("connectors.scopeHeld")}</FormLabel>
      <Select
        value={keyOf({ company, area })}
        onValueChange={(key) => {
          const scope = shown.find((s) => keyOf(s) === key);
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
          {shown.map((scope) => (
            <SelectItem key={keyOf(scope)} value={keyOf(scope)} className="font-mono">
              {scope.area ? keyOf(scope) : scope.company}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </FormItem>
  );
}
