import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ConnectorInstanceCard } from "@/features/integrations/connectors/connector-instance-card";
import type { ConnectorInstance } from "@/features/integrations/api";

const evidence: ConnectorInstance = {
  name: "gateway",
  connector: "evidence",
  enabled: true,
  scopeKind: "area",
  company: "acme",
  area: "cyber",
  hasToken: true,
  evidence: { baseUrl: "https://source.example.com", queries: 2 },
};

describe("ConnectorInstanceCard", () => {
  it("summarises an evidence source without offering an editor it does not have", () => {
    render(<ConnectorInstanceCard instance={evidence} />);

    expect(screen.getByText("https://source.example.com")).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});
