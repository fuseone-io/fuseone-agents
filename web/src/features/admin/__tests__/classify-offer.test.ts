import { describe, expect, it } from "vitest";
import { classifyOffer } from "@/features/admin/classify-offer";
import type { Me } from "@/features/session/api";

/*
Who is offered which ruling on a tool, decided per server (NT-019).

The platform's ceiling is the platform curator's; an owner's ruling is for a
server stored at a company or an area, by whoever may change that server. The
installation's servers and a parent company's are not an area curator's to
rule on, and they are not offered the button the server would refuse.
*/

const me = (platform: string[]): Me =>
  ({
    id: "usr_ana", display: "Ana", kind: "user", grants: [],
    can: ["tool:read", "provider:write"], platform,
    configure: [{ company: "acme", area: "cx" }],
  }) as Me;

const area = { scopeKind: "area", editable: true } as const;
const installation = { scopeKind: "installation", editable: false } as const;
const parentCompany = { scopeKind: "company", editable: false } as const;
const curator = me(["tool:read", "tool:classify", "provider:write"]);

describe("the ruling a caller is offered on a server's tools", () => {
  it.each([
    ["an area curator, their own area's server", me([]), area, "owner"],
    ["an area curator, the installation's server", me([]), installation, null],
    ["an area curator, the parent company's server", me([]), parentCompany, null],
    ["the platform curator, an area's server", curator, area, "choose"],
    ["the platform curator, the installation's server", curator, installation, "platform"],
    ["a platform reader who does not classify", me(["tool:read"]), installation, null],
    ["an area curator, a server that does not say", me([]), { scopeKind: "area" }, null],
    ["an open console, an area's server", null, area, "choose"],
    ["a session not answered yet", undefined, area, null],
  ] as const)("%s", (_case, caller, server, offer) => {
    expect(classifyOffer(caller, server)).toBe(offer);
  });
});
