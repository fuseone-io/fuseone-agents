import { useTranslation } from "react-i18next";
import type { Scope } from "@/features/scope/active-scope";
import {
  areaOptions,
  companyOptions,
  WHOLE_COMPANY,
} from "@/features/scope/scope-select-options";
import { ScopeSelect } from "@/features/scope/scope-select";
import { useScopeLists } from "@/features/scope/use-scope-lists";

/** How a form lays the pair out: ids, and the class of the two labels. */
export type CompanyAreaShape = {
  idPrefix: string;
  labelClass?: string;
};

/**
 * Where something applies: a company, then one of its areas or all of them.
 *
 * Chosen from what is registered rather than typed, so nothing can name an
 * area that does not exist and never meet the agents filed under the real one.
 * Two grid cells: the form around them decides the layout.
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
  const companies = companyOptions(data.companies, value.company, {
    scopes: data.scopes,
  });
  const areas = areaOptions(data.scopes, value.company, value.area);

  return (
    <>
      <ScopeSelect
        field={{
          id: `${shape.idPrefix}-company`,
          label: t("identity.company"),
          labelClass: shape.labelClass,
        }}
        value={value.company}
        options={companies}
        disabled={disabled || isLoading}
        onChange={(company) => onChange({ company, area: "" })}
      />
      <ScopeSelect
        field={{
          id: `${shape.idPrefix}-area`,
          label: t("identity.area"),
          labelClass: shape.labelClass,
        }}
        value={value.area || WHOLE_COMPANY}
        options={areas}
        disabled={disabled || isLoading || areas.length < 2}
        onChange={(area) =>
          onChange({
            company: value.company,
            area: area === WHOLE_COMPANY ? "" : area,
          })
        }
      />
    </>
  );
}
