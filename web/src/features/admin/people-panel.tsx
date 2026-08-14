import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Users } from "lucide-react";
import { Panel } from "@/components/shared/panel";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { Button } from "@/components/ui/button";
import { GrantEditor } from "@/features/admin/grant-editor";
import { LocalPersonForm } from "@/features/admin/local-person-form";
import { PersonRow } from "@/features/admin/person-row";
import { usePeople, type Person } from "@/features/admin/people-api";

/**
 * Who exists here, and what each one may do.
 *
 * Two things write these grants and the screen has to keep them apart: an
 * identity provider re-derives its own on every sign-in, so what it asserts is
 * shown and never offered for editing — the group is what to change, and a
 * revocation that undoes itself at the next sign-in is worse than no button.
 */
export function PeoplePanel() {
  const { t } = useTranslation();
  const { data, isLoading, error, refetch } = usePeople();
  const [editing, setEditing] = useState<Person | null>(null);
  const [adding, setAdding] = useState(false);

  const people = data?.items ?? [];

  if (editing) {
    return (
      <Panel title={t("people.editing", { name: editing.display })}>
        <GrantEditor person={editing} onDone={() => setEditing(null)} />
      </Panel>
    );
  }

  return (
    <Panel
      title={t("people.title")}
      action={
        <Button variant="outline" size="sm" onClick={() => setAdding(true)}>
          {t("people.addLocal")}
        </Button>
      }
    >
      {isLoading ? (
        <LoadingRows rows={4} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : people.length === 0 ? (
        <EmptyState
          icon={<Users className="size-6" />}
          title={t("people.emptyTitle")}
          hint={t("people.emptyHint")}
        />
      ) : (
        <ul className="flex flex-col gap-2">
          {people.map((person) => (
            <li key={person.id}>
              <PersonRow person={person} onEdit={() => setEditing(person)} />
            </li>
          ))}
        </ul>
      )}

      {adding && <LocalPersonForm onClose={() => setAdding(false)} />}
    </Panel>
  );
}
