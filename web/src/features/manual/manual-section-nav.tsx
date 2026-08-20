import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import type { ManualEntry } from "@/features/manual/api";
import { groupManualPages } from "@/features/manual/manual-search";

export function ManualSectionNav({ pages }: { pages: ManualEntry[] }) {
  const { t } = useTranslation();
  const groups = groupManualPages(pages);

  return (
    <nav className="sticky top-16 space-y-1" aria-label={t("manual.toc")}>
      {groups.map((group) => (
        <a
          key={group.section}
          href={`#${group.section}`}
          className="flex items-center justify-between gap-2 rounded-md px-3 py-2 text-sm text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          <span className="truncate">{t(`manual.sections.${group.section}`)}</span>
          <Badge variant="secondary">{group.pages.length}</Badge>
        </a>
      ))}
    </nav>
  );
}
