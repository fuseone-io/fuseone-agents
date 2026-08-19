import { useDeferredValue, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Users } from "lucide-react";
import { Panel } from "@/components/shared/panel";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { GrantEditor } from "@/features/admin/grant-editor";
import { LocalPersonForm } from "@/features/admin/local-person-form";
import { PasswordDialog } from "@/features/admin/password-dialog";
import { PeopleFooter } from "@/features/admin/people-footer";
import {
  isLocalPasswordIdentity,
  matchesPeopleView,
  matchesPerson,
  type PeopleView,
} from "@/features/admin/people-filters";
import { PeopleHeader } from "@/features/admin/people-header";
import { PeopleTableHeader } from "@/features/admin/people-table-header";
import { PeopleToolbar } from "@/features/admin/people-toolbar";
import { PersonRow } from "@/features/admin/person-row";
import { usePeople, type Person } from "@/features/admin/people-api";
import { useVisibleItems } from "@/hooks/use-visible-items";

const EMPTY_PEOPLE: Person[] = [];

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
  const [search, setSearch] = useState("");
  const [view, setView] = useState<PeopleView>("all");
  const [openRows, setOpenRows] = useState<Record<string, boolean>>({});

  const people = data?.items ?? EMPTY_PEOPLE;
  const query = useDeferredValue(search.trim().toLowerCase());
  const filtered = useMemo(
    () =>
      people.filter(
        (person) =>
          matchesPeopleView(person, view) &&
          (!query || matchesPerson(person, query, t)),
      ),
    [people, query, t, view],
  );
  const page = useVisibleItems(filtered, 50);
  const noMatches = people.length > 0 && filtered.length === 0;
  const noRole = people.filter((person) => (person.grants ?? []).length === 0);
  const local = people.filter(isLocalPasswordIdentity);

  if (editing) {
    return (
      <Panel title={t("people.editing", { name: editing.display })}>
        <GrantEditor person={editing} onDone={() => setEditing(null)} />
      </Panel>
    );
  }

  return (
    <section className="overflow-hidden rounded-xl border bg-card shadow-sm">
      <PeopleHeader onAddLocal={() => setAdding(true)} />

      {isLoading ? (
        <div className="p-4">
          <LoadingRows rows={4} />
        </div>
      ) : error ? (
        <div className="p-4">
          <ErrorState error={error} onRetry={() => void refetch()} />
        </div>
      ) : people.length === 0 ? (
        <div className="p-4">
          <EmptyState
            icon={<Users className="size-6" />}
            title={t("people.emptyTitle")}
            hint={t("people.emptyHint")}
          />
        </div>
      ) : (
        <>
          <PeopleToolbar
            search={search}
            onSearch={setSearch}
            view={view}
            onView={setView}
            noRole={noRole.length}
            local={local.length}
          />

          {noMatches ? (
            <div className="p-8">
              <EmptyState
                icon={<Users className="size-6" />}
                title={t("people.noMatches")}
                hint={t("people.noMatchesHint")}
              />
            </div>
          ) : (
            <ul className="divide-y divide-border-subtle">
              <PeopleTableHeader />
              {page.visible.map((person) => (
                <li key={person.id}>
                  <PersonRow
                    person={person}
                    open={Boolean(openRows[person.id])}
                    onOpenChange={(open) =>
                      setOpenRows((rows) => ({ ...rows, [person.id]: open }))
                    }
                    onEdit={() => setEditing(person)}
                    onSetPassword={() => setSettingPassword(person)}
                  />
                </li>
              ))}
            </ul>
          )}

          {!noMatches && (
            <PeopleFooter
              shown={page.loaded}
              total={people.length}
              hasMore={page.hasMore}
              onLoadMore={page.loadMore}
            />
          )}
        </>
      )}

      {adding && <LocalPersonForm onClose={() => setAdding(false)} />}
      {settingPassword && (
        <PasswordDialog
          person={settingPassword}
          onClose={() => setSettingPassword(null)}
        />
      )}
    </section>
  );
}
