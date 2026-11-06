import type { ConnectorEvidenceConfig } from "@/features/integrations/api";

/**
 * The two declarations from the manual, offered as a place to start. Vendors
 * appear here as examples only (PRD N8), on `example.com` hosts: the connector
 * holds no vendor, and these are text a person edits before saving.
 *
 * A test holds them equal to the manual, which the Go suite holds to the
 * validator, so an example cannot drift into something a save refuses.
 */
export const evidenceExamples: readonly {
  id: string;
  label: string;
  declaration: ConnectorEvidenceConfig;
}[] = [
  {
    id: "offset-periods",
    label: "connectors.evidenceExampleGateway",
    declaration: {
      baseUrl: "https://tenant.example.com",
      authHeader: "Netskope-Api-Token",
      control: "all_pages",
      rows: "result",
      refusal: {
        field: "ok",
        value: "0",
      },
      gap: {
        field: "bypass_traffic",
        value: "yes",
      },
      maxEvents: 200000,
      paging: {
        kind: "offset",
        limitParam: "limit",
        pageSize: 5000,
        offsetParam: "offset",
      },
      window: {
        kind: "period",
        periodParam: "timeperiod",
        periods: [3600, 86400, 604800, 2592000, 7776000],
      },
      queries: [
        {
          id: "all_pages",
          description: "Every page event, unfiltered. The positive control.",
          path: "/api/v2/events/data/page",
          fields: ["_id", "bypass_traffic"],
          idField: "_id",
        },
        {
          id: "pages_by_user",
          description: "One user's page events, with bytes.",
          path: "/api/v2/events/data/page",
          filters: [
            {
              name: "query",
              value: "user eq '{user}'",
            },
          ],
          parameters: [
            {
              name: "user",
              pattern: "[A-Za-z0-9._+-]+@[A-Za-z0-9.-]+",
            },
          ],
          fields: [
            "_id",
            "timestamp",
            "user",
            "hostname",
            "domain",
            "app",
            "appcategory",
            "client_bytes",
            "server_bytes",
            "bypass_traffic",
          ],
          idField: "_id",
          timeField: "timestamp",
        },
        {
          id: "uploads_by_user",
          description: "One user's uploads, with file names and no bytes.",
          path: "/api/v2/events/data/application",
          filters: [
            {
              name: "query",
              value: "user eq '{user}' and activity ~ 'Upload'",
            },
          ],
          parameters: [
            {
              name: "user",
              pattern: "[A-Za-z0-9._+-]+@[A-Za-z0-9.-]+",
            },
          ],
          fields: [
            "_id",
            "timestamp",
            "user",
            "app",
            "activity",
            "object",
            "object_type",
            "file_size",
            "bypass_traffic",
          ],
          idField: "_id",
          timeField: "timestamp",
        },
      ],
    },
  },
  {
    id: "cursor-range",
    label: "connectors.evidenceExampleEndpoint",
    declaration: {
      baseUrl: "https://console.example.com",
      authHeader: "Authorization",
      authPrefix: "ApiToken ",
      control: "all_activities",
      rows: "data",
      paging: {
        kind: "cursor",
        limitParam: "limit",
        pageSize: 1000,
        cursorParam: "cursor",
        nextCursor: "pagination.nextCursor",
      },
      window: {
        kind: "range",
        startParam: "createdAt__gte",
        endParam: "createdAt__lte",
        format: "iso",
      },
      queries: [
        {
          id: "all_activities",
          description:
            "Every console activity, unfiltered. The positive control.",
          path: "/web/api/v2.1/activities",
          fields: ["id"],
          idField: "id",
        },
        {
          id: "find_agent",
          description: "Finds a device by its exact name, to get its agent id.",
          path: "/web/api/v2.1/agents",
          filters: [
            {
              name: "computerName",
              value: "{host}",
            },
          ],
          parameters: [
            {
              name: "host",
              pattern: "[A-Za-z0-9._-]{1,64}",
            },
          ],
          fields: [
            "id",
            "computerName",
            "osName",
            "agentVersion",
            "lastActiveDate",
            "networkStatus",
            "siteName",
          ],
          idField: "id",
          windowless: true,
        },
        {
          id: "activities_by_agent",
          description: "One agent's console activities, including logons.",
          path: "/web/api/v2.1/activities",
          filters: [
            {
              name: "agentIds",
              value: "{agent}",
            },
          ],
          parameters: [
            {
              name: "agent",
              pattern: "[0-9]{1,32}",
            },
          ],
          fields: [
            "id",
            "createdAt",
            "activityType",
            "primaryDescription",
            "secondaryDescription",
          ],
          idField: "id",
          timeField: "createdAt",
        },
      ],
    },
  },
];
