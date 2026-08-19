import { useTranslation } from "react-i18next";

export function CompanyTableHeader() {
  const { t } = useTranslation();
  return (
    <li
      className="hidden h-8 grid-cols-[minmax(0,1.3fr)_112px_116px_36px] items-center gap-3 border-b bg-muted px-4 text-2xs font-semibold uppercase tracking-label text-text-disabled lg:grid"
      aria-hidden
    >
      <span>{t("companies.company")}</span>
      <span>{t("companies.areasColumn")}</span>
      <span>{t("companies.status")}</span>
      <span />
    </li>
  );
}
