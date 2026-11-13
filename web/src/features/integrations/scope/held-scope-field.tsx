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
import type { MeScope } from "@/features/session/api";
import type { ScopeValues } from "@/features/integrations/scope/scope-choice";

/** One of the scopes where the caller configures, the first by default. */
const keyOf = (s: MeScope) => `${s.company}/${s.area}`;

// Whether a grant at `held` reaches `scope`, as the server's Scope.Contains
// does: a company reaches its areas, an area only itself (NT-019).
const contains = (held: MeScope, scope: MeScope) =>
  held.company === scope.company && (held.area === "" || held.area === scope.area);

export function HeldScopeField({
  editing,
  scopes,
  label,
}: {
  editing: boolean;
  scopes: MeScope[];
  label: string;
}) {
  const { t } = useTranslation();
  const form = useFormContext<ScopeValues>();
  const choose = (scope: MeScope) => {
    form.setValue("scopeKind", scope.area ? "area" : "company");
    form.setValue("company", scope.company);
    form.setValue("area", scope.area);
  };
  const company = form.watch("company");
  const area = form.watch("area");
  useEffect(() => {
    const first = scopes[0];
    // A new form opens on the scope the console is reading, kept when a grant
    // reaches it, even one naming its company only. A scope from elsewhere,
    // such as an area under another grant, would be refused on save: the
    // first held scope replaces it.
    const current = { company, area };
    const held = scopes.some((s) => contains(s, current));
    if (!editing && held) choose(current);
    else if (!editing && first) choose(first);
    // Once, for a new row: a choice the person made is not undone.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  // The scope in the form is offered too when a grant covers it without
  // naming it, such as an area of a held company. That covers a row
  // being edited: only a row the caller may change opens, and that means
  // a held scope contains it.
  const current = { company, area };
  const listed = scopes.some((s) => keyOf(s) === keyOf(current));
  const shown =
    company && !listed && scopes.some((s) => contains(s, current))
      ? [current, ...scopes]
      : scopes;
  return (
    <FormItem>
      <FormLabel>{t(label)}</FormLabel>
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
