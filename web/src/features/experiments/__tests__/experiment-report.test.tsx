import { screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ExperimentReportView } from "@/features/experiments/experiment-report";
import { useExperiment } from "@/features/experiments/experiments-api";
import {
  experimentCase,
  renderWithClient,
  report,
  stubApi,
} from "@/features/experiments/__tests__/experiment-fixtures";

/*
Reading an experiment (NT-015): what it ran before what it found, and each case across
its repetitions — broken if any repetition broke, saying in how many.
*/

function Harness() {
  return <ExperimentReportView report={useExperiment("triage", "exp_triage_1")} />;
}

function showReport(cases = [experimentCase("estorno", 1, 5)]) {
  stubApi({ "GET /agents/triage/experiments/exp_triage_1": report(cases) });
  renderWithClient(<Harness />);
}

describe("an experiment's report", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("says what it ran before what it found", async () => {
    showReport();

    await screen.findByText("Versão do dataset");
    const labels = screen.getAllByRole("term").map((term) => term.textContent);
    expect(labels).toEqual([
      "Versão do dataset", "Versão do agente", "Configuração", "Casos rodados", "Apagados",
      "Repetições", "Seguraram", "Quebraram", "Custo",
    ]);
  });

  it("calls a case broken in one repetition broken, in 1 of 5", async () => {
    showReport();

    const row = (await screen.findByText("estorno")).closest("tr")!;
    expect(within(row).getByText("Quebrou")).toBeInTheDocument();
    expect(within(row).getByText("1 de 5")).toBeInTheDocument();
  });

  it("says a case that never ran did not run, rather than that it held", async () => {
    showReport([experimentCase("consulta", 0, 0)]);

    const row = (await screen.findByText("consulta")).closest("tr")!;
    expect(within(row).getByText("Não rodou")).toBeInTheDocument();
  });

  it("counts out of the repetitions that ran, and says which never opened", async () => {
    showReport([experimentCase("estorno", 0, 3)]);

    const row = (await screen.findByText("estorno")).closest("tr")!;
    expect(within(row).getByText("0 de 3")).toBeInTheDocument();
    expect(within(row).getByText("rodou 3 de 5 repetições")).toBeInTheDocument();
  });
});
