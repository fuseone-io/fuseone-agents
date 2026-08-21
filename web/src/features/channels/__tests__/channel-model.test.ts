import { describe, expect, it } from "vitest";
import { visibleChannels, type Channel } from "@/features/channels/channel-model";

const emptyChannel: Channel = {
  name: "acme-slack",
  kind: "slack",
  workspace: "Acme",
  deliveryMode: "socket",
  enabled: true,
  hasCredential: true,
  hasAppToken: true,
  conversations: [],
};

describe("channel filtering", () => {
  it("keeps a connected channel with no conversations in the default view", () => {
    expect(visibleChannels([emptyChannel], "", "all")).toEqual([emptyChannel]);
  });

  it("does not pretend an empty channel matches a conversation search", () => {
    expect(visibleChannels([emptyChannel], "alerts", "all")).toEqual([]);
  });
});
