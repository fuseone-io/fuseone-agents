import { ChevronDown, ChevronUp } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PersonAccessMatrix } from "@/features/admin/person-access-matrix";
import { PersonAccessSummary } from "@/features/admin/person-access-summary";
import { PersonActions } from "@/features/admin/person-actions";
import { groupGrants } from "@/features/admin/person-access-model";
import { PersonIdentityCell } from "@/features/admin/person-identity-cell";
import {
  PersonLastSeenCell,
  PersonSignInCell,
} from "@/features/admin/person-signin-cell";
import type { Person } from "@/features/admin/people-api";

/**
 * One person in the administrative list.
 *
 * The row is the summary and the expansion is the detail. All editing still
 * goes through the existing grant editor, so this screen does not grow a
 * second permission-writing path.
 */
export function PersonRow({
  person,
  open,
  onOpenChange,
  onEdit,
  onSetPassword,
}: {
  person: Person;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onEdit: () => void;
  onSetPassword: () => void;
}) {
  const groups = groupGrants(person.grants ?? []);
  const panelId = `person-access-${cssId(person.id)}`;

  return (
    <div className="min-w-0">
      <Button
        type="button"
        variant="ghost"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={() => onOpenChange(!open)}
        className="grid h-auto w-full grid-cols-1 justify-stretch gap-3 rounded-none px-4 py-3 text-left font-normal whitespace-normal transition-colors hover:bg-muted/60 lg:grid-cols-[minmax(0,1.1fr)_minmax(0,1.7fr)_138px_108px_36px] lg:items-center"
      >
        <PersonIdentityCell person={person} />
        <PersonAccessSummary groups={groups} />
        <PersonSignInCell person={person} />
        <PersonLastSeenCell lastSeen={person.lastSeen} />
        <span className="hidden size-7 place-items-center rounded-md text-muted-foreground lg:grid">
          {open ? (
            <ChevronUp className="size-4" aria-hidden />
          ) : (
            <ChevronDown className="size-4" aria-hidden />
          )}
        </span>
      </Button>

      {open && (
        <div id={panelId}>
          <PersonAccessMatrix groups={groups} onEdit={onEdit} />
          <PersonActions
            person={person}
            onEdit={onEdit}
            onSetPassword={onSetPassword}
          />
        </div>
      )}
    </div>
  );
}

function cssId(value: string) {
  return value.replace(/[^a-zA-Z0-9_-]/g, "-");
}
