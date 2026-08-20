import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import type { ManualPage } from "@/features/manual/api";

export function ManualPageOutline({ page }: { page: ManualPage }) {
  const { t } = useTranslation();
  if (page.headings.length === 0) return null;

  return (
    <aside className="hidden lg:block">
      <nav className="sticky top-16 space-y-1" aria-label={t("manual.onThisPage")}>
        <p className="px-3 pb-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
          {t("manual.onThisPage")}
        </p>
        {page.headings.map((heading) => (
          <a
            key={`${heading.id}-${heading.title}`}
            href={`#${heading.id}`}
            className={cn(
              "block rounded-md px-3 py-1.5 text-sm text-muted-foreground hover:bg-muted hover:text-foreground",
              heading.level > 2 && "pl-6 text-xs",
            )}
          >
            {heading.title}
          </a>
        ))}
      </nav>
    </aside>
  );
}
