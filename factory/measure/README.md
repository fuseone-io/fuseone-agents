# factory-measure

The software-factory pilot's **board measures** (#201, NT-013 § Measured during
the pilot): how long cards stay in each lane, who owned that time, and the
signals that a later stage or a person caught something a role let through.
The run ledger cannot see either one. It knows runs, approvals and cost, which
`GET /api/v1/work-items/measures` and the console's **Per role, per week**
panel report. That API lists `laneTime` and `caughtLater` in `unavailable[]`
as `board_only`, and this command is where they come from.

It is factory tooling, not part of the platform: nothing under `internal/`
imports it, and it ships in no image.

**It only reads.** The client sends GraphQL queries and REST `GET`s. It refuses
any GraphQL document that is not a `query` before sending it, writes nothing to
GitHub, and keeps no comment body. The one thing it takes from a comment is the
lane named on a `Return to:` last line.

**Do not rely on a report until the [canary](#the-canary) passes** with the
token the report will use.

## Usage

```sh
export GITHUB_TOKEN=…   # see Token below
go run ./factory/measure report --since 2026-10-05 --weeks 4 \
  --repo fuseone-io/fuseone-agents --project 2
go run ./factory/measure canary --issue 213 --pr 205 \
  --repo fuseone-io/fuseone-agents --project 2
```

| Flag | Command | Meaning |
|---|---|---|
| `--repo owner/name` | both | The repository. Required. |
| `--project N` | both | The organization project's number (the board is `orgs/<owner>/projects/N`). Required. |
| `--since YYYY-MM-DD` | report | The week holding this day is the first one reported. Required. |
| `--weeks N` | report | How many weeks. `0` (the default) runs through the current week. |
| `--issue N` | canary | An issue whose card a role's App moved. Required. |
| `--pr N` | canary | A pull request on the board, to check pull request cards too. |
| `--bots path` | both | A bot map to use instead of the shipped [`bots.yaml`](bots.yaml). |

The report goes to stdout as markdown. Exit codes: `0` done, `1` GitHub could
not be read or a canary check failed, `2` bad arguments or a bot map the report
cannot use. `GITHUB_GRAPHQL_URL` and `GITHUB_API_URL` point it at GitHub
Enterprise Server. By default they are `https://api.github.com/graphql` and
`https://api.github.com`.

## Token

`GITHUB_TOKEN`, from the environment only. It needs read access, nothing more:

| Token | Permissions |
|---|---|
| Fine-grained personal token, or a GitHub App installation token | Repository: **Metadata**, **Contents**, **Issues**, **Pull requests**, **Commit statuses**, all *read*. Organization: **Projects**, *read*. |
| Classic personal token | `repo` (or `public_repo` for a public repository) and `read:project`. |

A role's own App token works if its App has those permissions, but nothing
here needs write access, so do not use a token that has it when a read-only
one will do. Whatever token you choose, run the canary with that same token.

## The bot map

[`bots.yaml`](bots.yaml) maps each role to the login of its GitHub App's bot,
`<app-slug>[bot]`. It is versioned here and changed by pull request, like the
rest of `factory/`.

- An actor is a **role** only when GitHub reports it as a `Bot` whose login is
  exactly one in the map.
- An actor is a **person** when GitHub reports it as a `User`.
- Anything else is an **unmapped actor**: a `Bot` the map does not list (an
  App renamed, another integration, GitHub's own automation), a deleted
  account, a commit author with no linked account. It is listed in the
  report's *Unmapped actors* section, and none of its acts count for a role.
  Nothing is matched by a near name.

The map must list `triage`, `architect`, `developer`, `reviewer` and `qa`, each
with its own login. On 2026-11-03, `fuseone-factory-triage[bot]`,
`fuseone-factory-reviewer[bot]` and `fuseone-factory-qa[bot]` existed on GitHub
as Bot users. No App was found for the Architect or the Developer, so those
two are **placeholders**, and `report` refuses to run until they are filled in.
To check a slug:

```sh
gh api '/users/<app-slug>%5Bbot%5D' --jq .type   # Bot
```

## The canary

The measures rest on two GitHub premises that GitHub's documentation does not
settle:

1. the token can read `ProjectV2ItemStatusChangedEvent` on an issue timeline,
   along with the project the event belongs to;
2. a move an App makes (`move_card`) appears there as a `Bot` actor with the
   App's bot login.

`canary --issue N` checks both against an issue whose card a role's App moved.
With `--pr M` it also checks that a pull request card carries the same event
in `PullRequest.timelineItems`. It prints `PASS` or `FAIL` for each premise,
says what it found, and exits `1` if anything failed. Placeholders in the bot
map do not stop it. It names them, and a placeholder never matches.

**The report must not be relied on until the canary passes**, with the token
the report will use, on an issue a role's App actually moved.

On 2026-11-03, run with a person's token against `fuseone-io/fuseone-agents`,
project 2, issue #213 and pull request #205:

```
PASS  1. the token reads ProjectV2ItemStatusChangedEvent on an issue timeline: 2 status changes on #213 in project 2
FAIL  2. a move by a role's App appears as a Bot with the App's login: no move on #213 by a mapped App bot; moves by: User klinux (person) x2
PASS  3. a pull request card carries the same event in its own timeline: 2 status changes on #205 in project 2
```

The second premise is still open. No role's App had moved a card yet, and every
status change on the board was a person's. It has to be run again after the
first `move_card`, and with an App installation token if that is what the
report will use.

## What it reads

- every card on the project that is an issue or a pull request in the
  repository (draft items and other repositories' cards are skipped);
- every pull request updated since the window's first Monday;
- every issue or pull request those link to, followed until nothing new:
  `closingIssuesReferences` on a pull request, and
  `closedByPullRequestsReferences` (closed ones included) on an issue;
- their timelines through GraphQL, in batches of 20 aliases, each `first: 100`
  and followed by cursor to its last page: status changes, comments, labels,
  reopen, close and merge events, and, on pull requests, reviews and commits;
- through REST, every page of the commit statuses of each commit or reviewed
  head the timeline names, on pull requests a role wrote, acted on or labelled
  `factory:*`. Only `factory/qa` statuses are kept.

## The measures

Weeks start Monday 00:00 UTC, the same weeks the ledger's measures use. The
current week is marked *partial*. Times are in hours.

### Lane time

A **stay** is a card's time in a lane: from the status change that put it there
to the next status change on the same project. Done is where a card stops, so
it has no stays.

- A closed stay counts **in the week it ended**. A stay that ends on Sunday at
  23:59:59.999 UTC belongs to that week, and one that ends at Monday 00:00 to
  the next.
- **Open stays** (cards still in their lane) appear in no percentile. The
  *Open stays* section counts them per lane and gives the age of the oldest,
  measured to the moment the report was read.
- **Median** and **p90** are nearest-rank: the smallest stay with at least that
  share of the stays at or below it. With an even count, the median is the
  lower middle stay, never an average of two.

### Who owns the time

Each closed stay is split wherever a label that decides its owner changes,
and each piece goes to whoever had to move the card then:

| Lane | Owner |
|---|---|
| Triagem | the **Architect** while the issue carries `factory:needs-design`, **Triage** otherwise |
| Doing | from the pull request's sub-state label: `factory:needs-review` → **Reviewer**, `factory:needs-qa` → **QA**, `factory:changes-requested` → **Developer**, `factory:ready-to-merge` → a **person** |
| Preparado, Travado | a **person**: no role moves a card out of them |

In Doing, an issue card reads the labels of its linked pull requests. A pull
request card reads its own. With several sub-state labels on at once, the one
applied last decides. With none, the time is **unattributed**. That covers a
card in Doing before its pull request exists, and a card labelled
`factory:needs-runtime`. No rule above names an owner for either, so none is
guessed. The per-role table gives each owner's share of the closed stays that
ended that week, and the board moves it made that week.

### Caught later

A **signal** means a later stage or a person acted against something a role
let through. It is not a verdict that the role was wrong, and the report never
calls it a defect. Each signal counts in the week of the act that caught it.

| Role | Signal | Counted |
|---|---|---|
| Developer | **caught by the Reviewer: changes requested**: the Reviewer bot's `CHANGES_REQUESTED` review on a pull request the Developer bot opened | once per reviewed commit |
| Developer | **caught by QA on a head the Reviewer never reviewed**: `factory/qa` = `failure`, created by the QA bot, on a commit of a Developer pull request that the Reviewer bot left no review on | once per commit |
| Reviewer | **caught by QA on a head the Reviewer approved**: `factory/qa` = `failure`, created by the QA bot, on a commit the Reviewer bot approved. The Developer gets no signal for it | once per commit |
| Reviewer | **caught by a person after the Reviewer approved**: after an approval and before the Reviewer's next decision (`APPROVED`, `CHANGES_REQUESTED` or `DISMISSED`), a person's `CHANGES_REQUESTED` review or a person's commit | once per approval, at the first such act |
| QA | **caught by a person after QA passed**: after `factory/qa` = `success` by the QA bot and before QA's next status on the pull request, a person's `CHANGES_REQUESTED` review, a person's commit, the pull request closed without a merge, or a person reopening an issue it closes | once per success, at the first such act |
| Triage, Architect | **returned to Triagem by a person**: a person moved the card into Triagem from another lane | once per move |

A `factory/qa` failure on a head that the Reviewer had sent back gives nobody a
signal. A status that any creator other than the QA bot set on `factory/qa`
counts for nothing.

### Who a return to Triagem belongs to

A return goes to the role responsible for the card's **last block**, not to
the role that owns Triagem. The first rule that finds a role decides:

1. **The last `Return to:` comment.** Take the most recent comment written
   before the move by a role's bot whose last non-empty line is
   `Return to: <lane>` (the Travado protocol, [LANES.md](../LANES.md)), on the
   card, a pull request linked to it, or, for a pull request card, an issue it
   closes. That bot's role gets the signal.
2. **The last role write.** Failing that, take the most recent act by a role's
   bot before the move, in the same places: a comment, a review, a commit, a
   label or a board move.
3. **Nobody.** Failing that, the signal is unattributed.

The signal keeps the lane the card came back from and the rule that decided,
for example `returned to Triagem by a person from Preparado (by the last
Return to: comment)`. A return from Travado is usually the protocol working,
not a miss, and the from-lane keeps it apart from a return out of Preparado or
Doing.

**This attribution is approximate.** The last `Return to:` comment may belong
to an earlier block than the one the person was reacting to. For example, the
Architect blocked the card, a person answered, and then Triage passed it on.
Rule 1 still names the Architect. Rule 2 reads only who wrote last, not why.

## Limits

- **Approximations.** A commit's time is its committed date, because the
  timeline does not record when it was pushed. A commit's actor is the
  author's linked account, or an App bot read from its
  `<id>+<slug>[bot]@users.noreply.github.com` address. Any other author is
  unmapped. "The PR was closed without a merge" uses the pull request's merge
  state as read now. The return attribution above is approximate.
- **What is not read.** A pull request is read only if it was updated since
  the window opened or a card links to it. A commit a force-push removed from
  the pull request is checked only if a review names it. A card removed from
  the board has no stays. The project must belong to an organization.
- **Board history** exists only as `ProjectV2ItemStatusChangedEvent` on each
  item's timeline. The Projects API keeps no field history. This event was
  verified on this repository's issues and pull request cards on 2026-11-03,
  but it is a newer part of the timeline API than the rest, and GitHub does
  not promise to keep it stable. If GitHub changes or drops it, the canary is
  how you find out. The first status change of a card has an empty previous
  status: the card was added.
- **"Edited" is not measured anywhere.** The platform does not record a
  proposal approved with changes: the engine refuses arguments that differ from
  the approved ones. The ledger's measures report revised and repeated
  proposals after a refusal instead.
- **No partial reports.** If any query fails, the report fails, so it never
  shows a count with pages missing.
