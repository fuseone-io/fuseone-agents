import { describe, expect, it } from "vitest";
import {
  elasticsearchInstanceDefaults,
  elasticsearchInstancePayload,
  elasticsearchInstanceSchema,
} from "@/features/integrations/connectors/elasticsearch-instance-model";
import type { ConnectorInstance } from "@/features/integrations/api";

const stored: ConnectorInstance = {
  name: "gw-logs",
  connector: "elasticsearch",
  enabled: true,
  scopeKind: "area",
  company: "acme",
  area: "platform",
  hasToken: true,
  elasticsearch: {
    baseUrl: "http://search.internal:9200",
    username: "reader",
    index: "logs-*",
    pathField: "url.path",
    maxWindowDays: 14,
  },
};

function values() {
  return elasticsearchInstanceDefaults(stored, "acme", "platform");
}

describe("the elasticsearch instance model", () => {
  it("round-trips the configuration and never prefills the password", () => {
    const defaults = values();
    expect(defaults.index).toBe("logs-*");
    expect(defaults.pathField).toBe("url.path");
    expect(defaults.maxWindowDays).toBe(14);
    expect(defaults.token).toBe("");

    const payload = elasticsearchInstancePayload(defaults, true);
    expect(payload?.body.elasticsearch).toEqual({
      baseUrl: "http://search.internal:9200",
      username: "reader",
      index: "logs-*",
      pathField: "url.path",
      maxWindowDays: 14,
    });
    expect(payload?.body.token).toBeUndefined();
  });

  it("refuses to enable without a password anywhere", () => {
    expect(elasticsearchInstancePayload(values(), false)).toBeNull();
  });

  it("rejects an index that starts with a wildcard", () => {
    const result = elasticsearchInstanceSchema.safeParse({
      ...values(),
      index: "*",
    });
    expect(result.success).toBe(false);
  });
});
