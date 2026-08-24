import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { EgressDenialsPanel } from "@/features/runtime/egress-denials-panel";

test("names stable stdio egress codes instead of showing raw codes", () => {
  render(
    <EgressDenialsPanel
      denials={[
        {
          code: "stdio_egress_metadata_refused",
          attempts: 3,
          servers: 1,
          destinations: 2,
          firstAt: "2026-08-24T11:00:00.000Z",
          lastAt: "2026-08-24T12:00:00.000Z",
        },
      ]}
    />,
  );

  expect(
    screen.getByText("Destino de metadata de nuvem ou link-local recusado"),
  ).toBeInTheDocument();
  expect(screen.queryByText("stdio_egress_metadata_refused")).not.toBeInTheDocument();
  expect(screen.getByText("3")).toBeInTheDocument();
  expect(screen.getByText("1")).toBeInTheDocument();
  expect(screen.getByText("2")).toBeInTheDocument();
});
