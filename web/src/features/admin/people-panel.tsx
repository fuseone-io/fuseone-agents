import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Users } from "lucide-react";
import { Panel } from "@/components/shared/panel";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { LoadMore } from "@/components/shared/load-more";
import { Button } from "@/components/ui/button";
import { GrantEditor } from "@/features/admin/grant-editor";
import { LocalPersonForm } from "@/features/admin/local-person-form";
import { PasswordDialog } from "@/features/admin/password-dialog";
import { PersonRow } from "@/features/admin/person-row";
import { usePeople, type Person } from "@/features/admin/people-api";
import { useVisibleItems } from "@/hooks/use-visible-items";

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
  const [settingPassword, setSettingPassword] = useState<Person | null>(null);

  const people = data?.items ?? [];
  const page = useVisibleItems(people, 50);

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
        <>
          <ul className="flex flex-col gap-2">
            {page.visible.map((person) => (
              <li key={person.id}>
                <PersonRow
                  person={person}
                  onEdit={() => setEditing(person)}
                  onSetPassword={() => setSettingPassword(person)}
                />
              </li>
            ))}
          </ul>
          <LoadMore
            loaded={page.loaded}
            total={page.total}
            hasMore={page.hasMore}
            isLoading={false}
            onLoad={page.loadMore}
          />
        </>
      )}

      {adding && <LocalPersonForm onClose={() => setAdding(false)} />}
      {settingPassword && (
        <PasswordDialog
          person={settingPassword}
          onClose={() => setSettingPassword(null)}
        />
      )}
    </Panel>
  );
}
