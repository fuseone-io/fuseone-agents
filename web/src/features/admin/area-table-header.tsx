import { useTranslation } from "react-i18next";

export function AreaTableHeader() {
  const { t } = useTranslation();
  return (
    <li
      className="hidden h-8 grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)_112px_36px] items-center gap-3 border-b bg-muted px-4 text-2xs font-semibold uppercase tracking-label text-text-disabled lg:grid"
      aria-hidden
    >
      <span>{t("admin.area")}</span>
      <span>{t("admin.company")}</span>
      <span>{t("admin.actions")}</span>
      <span />
    </li>
  );
}
