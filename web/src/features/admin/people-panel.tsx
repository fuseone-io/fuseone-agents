import { useDeferredValue, useMemo, useState } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { Users } from "lucide-react";
import { Panel } from "@/components/shared/panel";
import { Toolbar } from "@/components/shared/toolbar";
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

  const people = data?.items ?? EMPTY_PEOPLE;
  const query = useDeferredValue(search.trim().toLowerCase());
  const filtered = useMemo(
    () =>
      query
        ? people.filter((person) => matchesPerson(person, query, t))
        : people,
    [people, query, t],
  );
  const page = useVisibleItems(filtered, 50);
  const noMatches = people.length > 0 && filtered.length === 0;

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
        <div className="flex flex-col gap-3">
          <Toolbar
            placeholder="people.search"
            value={search}
            onChange={setSearch}
            trailing={
              <span className="text-xs text-muted-foreground">
                {t("people.showing", {
                  shown: filtered.length,
                  total: people.length,
                })}
              </span>
            }
          />
          {noMatches ? (
            <EmptyState
              icon={<Users className="size-6" />}
              title={t("people.noMatches")}
              hint={t("people.noMatchesHint")}
            />
          ) : (
            <ul className="flex flex-col gap-2">
              <li
                className="hidden grid-cols-[minmax(0,1.1fr)_minmax(0,1.5fr)_minmax(128px,auto)_minmax(136px,auto)] gap-3 px-3 text-2xs font-medium uppercase tracking-normal text-muted-foreground lg:grid"
                aria-hidden
              >
                <span>{t("people.person")}</span>
                <span>{t("people.access")}</span>
                <span>{t("people.lastActivity")}</span>
                <span>{t("people.actions")}</span>
              </li>
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
          )}
          {!noMatches && (
            <LoadMore
              loaded={page.loaded}
              total={page.total}
              hasMore={page.hasMore}
              isLoading={false}
              onLoad={page.loadMore}
            />
          )}
        </div>
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

function matchesPerson(person: Person, query: string, t: TFunction) {
  return [
    person.display,
    person.email,
    person.id,
    person.provider,
    person.username,
    person.kind,
    t(`people.kind.${person.kind}`),
    ...(person.grants ?? []).flatMap((grant) => [
      grant.role,
      t(`roles.${grant.role}`),
      grant.company,
      grant.area,
      grant.asserted ? "provider" : "direct",
      grant.asserted ? t("people.asserted") : t("people.grantedHere"),
    ]),
  ]
    .filter(Boolean)
    .some((value) => String(value).toLowerCase().includes(query));
}
