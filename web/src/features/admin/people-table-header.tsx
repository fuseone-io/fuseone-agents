import { useTranslation } from "react-i18next";

export function PeopleTableHeader() {
  const { t } = useTranslation();
  return (
    <li
      className="hidden h-8 grid-cols-[minmax(0,1.1fr)_minmax(0,1.7fr)_138px_108px_36px] items-center gap-3 border-b bg-muted px-4 text-2xs font-semibold uppercase tracking-label text-text-disabled lg:grid"
      aria-hidden
    >
      <span>{t("people.person")}</span>
      <span>{t("people.access")}</span>
      <span>{t("people.signIn")}</span>
      <span>{t("people.lastSeenColumn")}</span>
      <span />
    </li>
  );
}
