import { Link } from "react-router-dom";
import { workItemPath } from "@/features/work-items/work-item-path";

/**
 * A claimed reference in a run's trail, leading to the card's own trail:
 * every run that worked on it, not only this one.
 */
export function WorkItemRefLink({
  reference,
  label,
}: {
  reference: string;
  label: string;
}) {
  return (
    <Link
      to={workItemPath(reference)}
      className="rounded-sm underline underline-offset-4 hover:text-foreground focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/26"
    >
      {label}
    </Link>
  );
}
