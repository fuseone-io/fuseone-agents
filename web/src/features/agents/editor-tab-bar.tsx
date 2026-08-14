import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import {
  HINTS,
  LABELS,
  TABS,
  type EditorTab,
} from "@/features/agents/editor-tabs";

/**
 * Four decisions, one at a time.
 *
 * The screen used to be one column holding six unrelated decisions and about
 * fifty controls, none of them related to its neighbour. That is not too much
 * content; it is a missing decomposition.
 *
 * Each tab carries a live count, and the count is the point: it says where the
 * substance is before anybody clicks. Beside them sits one line naming what
 * the open tab decides — a heading answers "where am I", and this answers
 * "what am I being asked".
 */
export function EditorTabBar({
  active,
  onChange,
  counts,
}: {
  active: EditorTab;
  onChange: (tab: EditorTab) => void;
  counts: Record<EditorTab, number>;
}) {
  const { t } = useTranslation();

  return (
    <div
      role="tablist"
      aria-label={t("agents.editorSections")}
      className="flex h-11 items-center gap-1 border-b border-border px-4"
    >
      {TABS.map((tab) => (
        <button
          key={tab}
          role="tab"
          type="button"
          aria-selected={active === tab}
          onClick={() => onChange(tab)}
          // Borderless and the same height in both states, so activating one
          // never shifts the row underneath it.
          className={cn(
            "flex h-11 items-center gap-2 px-3 text-sm transition-colors",
            active === tab
              ? "text-foreground shadow-[inset_0_-2px_0_var(--primary)]"
              : "text-muted-foreground hover:text-foreground",
          )}
        >
          {t(LABELS[tab])}
          <span
            className={cn(
              "rounded-pill px-1.5 text-2xs tabular-nums",
              active === tab
                ? "bg-surface-accent text-text-accent"
                : "bg-muted text-muted-foreground",
            )}
          >
            {counts[tab]}
          </span>
        </button>
      ))}

      <p className="ml-auto hidden truncate text-2xs text-muted-foreground lg:block">
        {t(HINTS[active])}
      </p>
    </div>
  );
}
