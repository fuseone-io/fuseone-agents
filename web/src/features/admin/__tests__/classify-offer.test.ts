import { describe, expect, it } from "vitest";
import { classifyOffer } from "@/features/admin/classify-offer";
import type { Me, MeScope } from "@/features/session/api";

/*
Who is offered which ruling on a tool, decided per tool (NT-019).

The platform's ceiling is the platform curator's. An owner's ruling is for a
tool whose server is stored at a company or an area, by whoever holds
tool:classify at a scope containing it: a company contains its areas, an area
only itself. The installation's tools and a sibling's are not offered, and
neither is anything to a caller who configures servers but does not classify.
*/

const me = (classify: MeScope[], platform: string[] = []): Me =>
  ({
    id: "usr_ana", display: "Ana", kind: "user", grants: [],
    can: ["tool:read", "provider:write", "tool:classify"], platform,
    configure: [{ company: "acme", area: "cx" }], classify,
  }) as Me;

const tool = (company: string, area = "") => ({ scope: { company, area } });
const cx = { company: "acme", area: "cx" };
const acme = { company: "acme", area: "" };
const curator = me([], ["tool:read", "tool:classify", "provider:write"]);

describe("the ruling a caller is offered on one tool", () => {
  it.each([
    ["an area curator, their own area's tool", me([cx]), tool("acme", "cx"), "owner"],
    ["a company curator, an area's tool in it", me([acme]), tool("acme", "cx"), "owner"],
    ["a company curator, the company's own tool", me([acme]), tool("acme"), "owner"],
    ["an area curator, a sibling area's tool", me([cx]), tool("acme", "fin"), null],
    ["an area curator, the company's tool", me([cx]), tool("acme"), null],
    ["an area curator, the installation's tool", me([cx]), tool("*"), null],
    ["a server administrator who does not classify", me([]), tool("acme", "cx"), null],
    ["a tool that does not say where it is", me([cx]), {}, null],
    ["the platform curator, an area's tool", curator, tool("acme", "cx"), "choose"],
    ["the platform curator, the installation's tool", curator, tool("*"), "platform"],
    ["a platform reader who does not classify", me([], ["tool:read"]), tool("*"), null],
    ["an open console, an area's tool", null, tool("acme", "cx"), "choose"],
    ["a session not answered yet", undefined, tool("acme", "cx"), null],
  ] as const)("%s", (_case, caller, one, offer) => {
    expect(classifyOffer(caller, one)).toBe(offer);
  });
});
