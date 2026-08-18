import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ThroughputPanel } from "@/features/overview/throughput-panel";
import { i18next } from "@/i18n";

const overview = vi.hoisted(() => ({
  useThroughput: vi.fn(),
}));

vi.mock("@/features/overview/api", () => ({
  useThroughput: overview.useThroughput,
}));

vi.mock("@/features/overview/throughput-chart", () => ({
  ThroughputChart: () => <div data-testid="throughput-chart" />,
}));

describe("the throughput legend", () => {
  beforeEach(async () => {
    await i18next.changeLanguage("en-US");
    overview.useThroughput.mockReset();
    overview.useThroughput.mockReturnValue({
      data: { buckets: [] },
      isLoading: false,
      error: null,
    });
  });

  afterEach(async () => {
    cleanup();
    await i18next.changeLanguage("pt-BR");
  });

  it("uses the selected locale instead of inline Portuguese labels", () => {
    render(<ThroughputPanel since="2026-08-18T00:00:00.000Z" />);

    expect(screen.getByText("finished")).toBeInTheDocument();
    expect(screen.getByText("in progress")).toBeInTheDocument();
    expect(screen.getByText("stopped")).toBeInTheDocument();
    expect(screen.queryByText("em curso")).not.toBeInTheDocument();
    expect(screen.queryByText("paradas")).not.toBeInTheDocument();
  });
});
