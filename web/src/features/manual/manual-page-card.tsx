import { Link } from "react-router-dom";
import { Badge } from "@/components/ui/badge";
import type { ManualEntry } from "@/features/manual/api";

export function ManualPageCard({ page }: { page: ManualEntry }) {
  return (
    <Link
      to={`/manual/${page.slug}`}
      className="block min-w-0 rounded-lg border border-border bg-card p-4 shadow-sm transition-colors hover:border-primary"
    >
      <div className="flex min-w-0 items-start justify-between gap-3">
        <h2 className="min-w-0 font-medium break-words">{page.title}</h2>
        <Badge variant="secondary">{page.headings.length}</Badge>
      </div>
      <p className="mt-1 text-sm text-muted-foreground break-words">{page.summary}</p>
      <div className="mt-3 flex flex-wrap gap-1.5">
        {page.tags.slice(0, 4).map((tag) => (
          <Badge key={tag} variant="outline" className="text-[11px]">
            {tag}
          </Badge>
        ))}
      </div>
    </Link>
  );
}
