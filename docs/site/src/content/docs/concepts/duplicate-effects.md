---
title: Duplicate effects
description: How FuseOne avoids repeating the same effect across runs.
---

Duplicate-effect recognition is not cache.

Cache reuses a tool response. Duplicate-effect recognition skips an effect
that the platform already recorded as done for the same company, area, agent,
tool and semantic fingerprint.

## Semantic keys

A Curator declares which argument paths identify the effect. FuseOne hashes
only those values, with the platform's scope prefix around them. There is no
fallback to "hash all arguments" because correlation IDs, timestamps and
irrelevant fields would defeat the mechanism.

The model does not choose the tenant scope. Company, area, agent and tool are
prefixes controlled by the platform.

## Runtime flow

The engine reads duplicate state before the Gate, lets the Gate decide, then
reserves the fingerprint only after the proposed effect is executable. A
successful tool return confirms the fingerprint. A failed tool return releases
the reservation.

If another run is already executing the same effect, the waiting run retries
with a stable `dedupe_in_flight` reason instead of pretending a human needs to
intervene.

## Related manual page

- [Duplicate effect recognition](../../manual/en-us/duplicate-effects/)
