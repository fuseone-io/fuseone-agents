import { describe, expect, it } from "vitest";
import { ApiError, unwrap } from "@/lib/api/client";

const replyOf = (status: number) => new Response(null, { status });

describe("unwrap", () => {
  it("accepts a 204, because a reply with no content is still a success", () => {
    expect(unwrap({ response: replyOf(204) })).toBeUndefined();
  });

  it("rejects a 403 even though the server sent no problem body", () => {
    expect(() => unwrap({ response: replyOf(403) })).toThrow(ApiError);
  });

  it("carries the problem title, so a view can say what actually failed", () => {
    const call = () =>
      unwrap({
        response: replyOf(409),
        error: { title: "Esse escopo já tem um teto", status: 409 },
      });
    expect(call).toThrow("Esse escopo já tem um teto");
  });
});
