import { useTranslation } from "react-i18next";
import type { ReactNode } from "react";
import { Search } from "lucide-react";

/**
 * The console's filter bar: a search field that gives way to the filters
 * beside it, at the same 32px height as everything else in a dense screen.
 *
 * The field is a plain input rather than the shadcn `Input` because the design
 * puts the icon inside the box and sizes the whole thing to the row rhythm.
 * It carries a label for the same reason every other control does — a
 * placeholder disappears the moment somebody types.
 */
export function Toolbar({
  placeholder,
  value,
  onChange,
  children,
}: {
  placeholder: string;
  value: string;
  onChange: (value: string) => void;
  children?: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex shrink-0 items-center gap-2.5">
      <label className="flex h-8 max-w-[320px] flex-1 items-center gap-[7px] rounded-sm border border-input bg-card px-2.5 focus-within:shadow-[var(--elevation-focus)]">
        <Search
          className="size-3.5 shrink-0 text-muted-foreground"
          aria-hidden
        />
        <span className="sr-only">{t(placeholder)}</span>
        <input
          type="search"
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder={t(placeholder)}
          className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
        />
      </label>
      {children}
    </div>
  );
}
