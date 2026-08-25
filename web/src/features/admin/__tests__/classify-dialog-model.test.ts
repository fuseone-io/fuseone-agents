import { describe, expect, it } from "vitest";
import {
  classificationInput,
  startRulingFromTool,
} from "@/features/admin/classify-dialog-model";
import type { Tool } from "@/features/admin/api";

function tool(over: Partial<Tool> = {}): Tool {
  return {
    toolId: "crm.delete_account",
    server: "crm",
    effect: "destructive",
    untrusted: false,
    digest: "sha-1",
    ...over,
  };
}

describe("classification dialog model", () => {
  it("will not build a submission for enabled dedupe without a semantic key", () => {
    const ruling = {
      ...startRulingFromTool(tool()),
      dedupe: { enabled: true, windowSeconds: "3600", argPaths: "" },
    };

    expect(classificationInput(tool(), ruling)).toBeNull();
  });

  it("omits dedupe when duplicate-effect recognition is not enabled", () => {
    const input = classificationInput(tool(), startRulingFromTool(tool()));

    expect(input).toMatchObject({ effect: "destructive" });
    expect(input).not.toHaveProperty("dedupe");
  });
});
