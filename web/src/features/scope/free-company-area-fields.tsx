import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import type { Scope } from "@/features/scope/active-scope";
import type { RegisteredScope } from "@/features/scope/api";
import type { CompanyAreaShape } from "@/features/scope/company-area-fields";
import {
  ScopeSuggestField,
  type ScopeSuggestion,
} from "@/features/scope/scope-suggest-field";

const INSTALLATION = "*";

/**
 * The company and area typed, for whoever may place things at any scope but
 * may not list every company. Closing the choice to the companies their
 * areas show would forbid what the server allows; the areas they do see
 * are suggested instead.
 */
export function FreeCompanyAreaFields({
  value,
  shape,
  scopes,
  disabled,
  onChange,
}: {
  value: Scope;
  shape: CompanyAreaShape;
  scopes: RegisteredScope[] | undefined;
  disabled: boolean;
  onChange: (next: Scope) => void;
}) {
  const { t } = useTranslation();
  const mode = shape.area ?? "whole";
  const field = (name: "company" | "area") => ({
    id: `${shape.idPrefix}-${name}`,
    label: t(`identity.${name}`),
    labelClass: shape.labelClass,
    error: shape.errors?.[name],
    suggest: t(name === "company" ? "scope.suggestCompanies" : "scope.suggestAreas"),
  });
  return (
    <>
      <ScopeSuggestField
        field={field("company")}
        value={value.company}
        suggestions={companySuggestions(scopes, shape.installation === true, t)}
        disabled={disabled}
        // Another company empties the area, as the closed selects do: an area
        // typed for one company is not carried over to the next.
        onChange={(company) => onChange({ company, area: company === value.company ? value.area : "" })}
      />
      {mode !== "none" && (
        <ScopeSuggestField
          field={{
            ...field("area"),
            placeholder: mode === "whole" ? t("scope.optionWholeCompany") : undefined,
          }}
          value={value.area}
          suggestions={areaSuggestions(scopes, value.company)}
          disabled={disabled}
          onChange={(area) => onChange({ company: value.company, area })}
        />
      )}
    </>
  );
}

function companySuggestions(
  scopes: RegisteredScope[] | undefined,
  installation: boolean,
  t: TFunction,
): ScopeSuggestion[] {
  const seen = [...new Set((scopes ?? []).map((s) => s.company))];
  const companies = seen.map((id) => ({ value: id, label: id }));
  return installation
    ? [{ value: INSTALLATION, label: t("scope.optionWholeInstallation") }, ...companies]
    : companies;
}

function areaSuggestions(
  scopes: RegisteredScope[] | undefined,
  company: string,
): ScopeSuggestion[] {
  return (scopes ?? [])
    .filter((s) => s.company === company.trim())
    .map((s) => ({ value: s.area, label: s.area, hint: s.label }));
}
