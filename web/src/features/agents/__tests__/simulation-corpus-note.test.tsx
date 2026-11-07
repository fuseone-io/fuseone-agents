import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { SimulationCorpusNote } from "@/features/agents/simulation-corpus-note";
import { setLocale } from "@/i18n";

const report = { id: "sim-1", cases: [], running: false };

describe("SimulationCorpusNote", () => {
  it.each([
    ["en-US", "Read against corpus version 3.", "Not checked yet"],
    ["pt-BR", "Lida contra a versão 3 do corpus.", "Ainda não verificados"],
  ] as const)("names the version and what it did not check, in %s", (locale, version, title) => {
    setLocale(locale);
    render(
      <SimulationCorpusNote
        report={{ ...report, datasetVersion: 3, notChecked: ["reg-novo"] }}
      />,
    );
    expect(screen.getByText(version)).toBeInTheDocument();
    expect(screen.getByText(title)).toBeInTheDocument();
    expect(screen.getByText("reg-novo")).toBeInTheDocument();
  });

  it.each([
    ["en-US", "No corpus version"],
    ["pt-BR", "Sem versão do corpus"],
  ] as const)("says a battery has no version, in %s", (locale, label) => {
    setLocale(locale);
    render(<SimulationCorpusNote report={{ ...report, unpinned: true }} />);
    expect(screen.getByText(label)).toBeInTheDocument();
  });

  it.each([
    ["en-US", "Lost since"],
    ["pt-BR", "Perdidos depois"],
  ] as const)("names what was lost after the battery, in %s", (locale, title) => {
    setLocale(locale);
    render(<SimulationCorpusNote report={{ ...report, erasedSince: ["sumiu"] }} />);
    expect(screen.getByText(title)).toBeInTheDocument();
    expect(screen.getByText("sumiu")).toBeInTheDocument();
  });

  it("says nothing about an uploaded set", () => {
    setLocale("en-US");
    const { container } = render(<SimulationCorpusNote report={report} />);
    expect(container).toBeEmptyDOMElement();
  });
});
