import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { setLocale } from "@/i18n";
import { WorkbenchNotice } from "@/features/workbench/workbench-notice";

/*
What a refused save says it saved. Each operation lands on its own, so a
refusal after the first says how many did, never that nothing was saved.
*/
describe("the refusal notice", () => {
  beforeEach(() => setLocale("pt-BR"));

  it("says nothing was saved when nothing was", () => {
    render(<WorkbenchNotice state="stale" landed={{ landed: 0, total: 2 }} onReread={() => {}} />);
    expect(screen.getByText(/Nada foi salvo/)).toBeInTheDocument();
  });

  it("says how much landed when part of the save did", () => {
    render(<WorkbenchNotice state="stale" landed={{ landed: 1, total: 2 }} onReread={() => {}} />);
    expect(screen.getByText(/1 de 2 alterações foram salvas/)).toBeInTheDocument();
    expect(screen.queryByText(/Nada foi salvo/)).toBeNull();
  });

  it("says it in English too", () => {
    setLocale("en-US");
    render(<WorkbenchNotice state="gone" landed={{ landed: 1, total: 3 }} onReread={() => {}} />);
    expect(screen.getByText(/1 of 3 changes were saved/)).toBeInTheDocument();
    setLocale("pt-BR");
  });
});
