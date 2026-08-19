import type { TFunction } from "i18next";
import type { Person } from "@/features/admin/people-api";

export const PEOPLE_VIEWS = ["all", "provider", "local", "noRole"] as const;

export type PeopleView = (typeof PEOPLE_VIEWS)[number];

export function matchesPeopleView(person: Person, view: PeopleView) {
  if (view === "all") return true;
  if (view === "provider") return isProviderIdentity(person);
  if (view === "local") return isLocalPasswordIdentity(person);
  return (person.grants ?? []).length === 0;
}

export function isProviderIdentity(person: Person) {
  return Boolean(person.provider) && person.kind === "user";
}

export function isLocalPasswordIdentity(person: Person) {
  return (
    person.kind === "user" &&
    Boolean(person.username) &&
    !(person.provider ?? "").startsWith("oidc")
  );
}

export function matchesPerson(person: Person, query: string, t: TFunction) {
  return [
    person.display,
    person.email,
    person.id,
    person.provider,
    person.username,
    person.kind,
    t(`people.kind.${person.kind}`),
    isProviderIdentity(person) ? t("people.providerSignIn") : undefined,
    isLocalPasswordIdentity(person) ? t("people.localPassword") : undefined,
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
