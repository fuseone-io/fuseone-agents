import { describe, expect, it } from "vitest";
import { pathOfScope } from "@/features/admin/budget-scope";

describe("a ceiling's path", () => {
  // Typed in free mode, each part loses its own outer spaces before the two
  // are joined: " acme " and " cx " are acme/cx, never "acme / cx".
  it("trims the company and the area before joining them", () => {
    expect(pathOfScope({ company: " acme ", area: " cx " })).toBe("acme/cx");
    expect(pathOfScope({ company: " acme ", area: "  " })).toBe("acme");
    expect(pathOfScope({ company: "*", area: "" })).toBe("installation");
  });
});
