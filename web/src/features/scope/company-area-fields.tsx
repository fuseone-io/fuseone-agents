import { useTranslation } from "react-i18next";
import type { Scope } from "@/features/scope/active-scope";
import {
  areaOptions,
  companyOptions,
  WHOLE_COMPANY,
} from "@/features/scope/scope-select-options";
import { ScopeSelect } from "@/features/scope/scope-select";
import { useScopeLists } from "@/features/scope/use-scope-lists";

/**
 * How a form lays the pair out.
 *
 * `area` says what the area select is for: "whole" offers the whole company
 * beside its areas, "required" asks for one area, and "none" leaves the area
 * select out. `errors` are i18n keys the form's validation gave.
 */
export type CompanyAreaShape = {
  idPrefix: string;
  labelClass?: string;
  area?: "whole" | "required" | "none";
  errors?: { company?: string; area?: string };
};

/**
 * Where something applies: a company, then one of its areas or all of them.
 *
 * Chosen from what is registered rather than typed, so nothing can name an
 * area that does not exist and never meet the agents filed under the real one.
 * One grid cell per select: the form around them decides the layout.
 */
export function CompanyAreaFields({
  value,
  shape,
  disabled = false,
  onChange,
}: {
  value: Scope;
  shape: CompanyAreaShape;
  disabled?: boolean;
  onChange: (next: Scope) => void;
}) {
  const { t } = useTranslation();
  const { data, isLoading } = useScopeLists();
  const mode = shape.area ?? "whole";
  const companies = companyOptions(data.companies, value.company, {
    scopes: data.scopes,
  });
  const areas = areaOptions(data.scopes, value.company, value.area).filter(
    (option) => mode === "whole" || option.state !== "whole",
  );
  // The whole company alone is no choice; an area to require needs one.
  const choosable = areas.length > (mode === "whole" ? 1 : 0);
  const field = (name: "company" | "area") => ({
    id: `${shape.idPrefix}-${name}`,
    label: t(`identity.${name}`),
    labelClass: shape.labelClass,
    error: shape.errors?.[name],
  });

  return (
    <>
      <ScopeSelect
        field={field("company")}
        value={value.company}
        options={companies}
        disabled={disabled || isLoading}
        onChange={(company) => onChange({ company, area: "" })}
      />
      {mode !== "none" && (
        <ScopeSelect
          field={field("area")}
          value={mode === "whole" ? value.area || WHOLE_COMPANY : value.area}
          options={areas}
          disabled={disabled || isLoading || !choosable}
          onChange={(area) =>
            onChange({
              company: value.company,
              area: area === WHOLE_COMPANY ? "" : area,
            })
          }
        />
      )}
    </>
  );
}
