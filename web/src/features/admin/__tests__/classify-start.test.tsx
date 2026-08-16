import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ClassifyDialog } from "@/features/admin/classify-dialog";
import type { Tool } from "@/features/admin/api";

function tool(over: Partial<Tool> = {}): Tool {
  return {
    toolId: "crm.delete_account",
    server: "crm",
    effect: "destructive",
    untrusted: false,
    digest: "sha-1",
    ...over,
  };
}

function open(one: Tool | null) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ClassifyDialog tool={one} tools={[]} onClose={vi.fn()} />
    </QueryClientProvider>,
  );
}

/*
Changing a ruling starts from the ruling.

Opened blank, the safest-looking answer — read — is the one a distracted person
submits by touching nothing, and a destructive tool quietly becomes a read.
*/
describe("what a ruling opens on", () => {
  it("starts a tool that was already judged on what it was judged as", () => {
    open(tool());
    // The Select shows what is chosen; a blank one shows the placeholder.
    expect(screen.getByLabelText("O que esta ferramenta faz com o mundo")).toHaveTextContent(/destrutiv/i);
  });

  /*
   * Asserting it is not destructive proved nothing: the value it used to open
   * on was `read`, which is the permissive one and the one this had to catch.
   * A test written as "not the dangerous answer" passes on the answer that
   * grants.
   */
  it("starts a tool nobody has judged with no effect at all", () => {
    open(tool({ effect: "unknown" }));
    const chooser = screen.getByLabelText("O que esta ferramenta faz com o mundo");
    expect(chooser).toHaveTextContent(/diga o que ela faz/i);
    for (const named of [/leitura/i, /escrita/i, /destrutiv/i, /financeir/i]) {
      expect(chooser).not.toHaveTextContent(named);
    }
  });

  // And it cannot be recorded until somebody says. A disabled button asks the
  // question again; a default answers it with whatever the form held.
  it("will not record a ruling that names no effect", () => {
    open(tool({ effect: "unknown" }));
    expect(screen.getByRole("button", { name: /registrar|gravar|classificar/i })).toBeDisabled();
  });

  /*
   * A tool whose definition moved is a different thing to judge, and carrying
   * the previous answers into it is the exact mistake the digest exists to
   * prevent.
   */
  it("does not carry one tool's answers into the next", () => {
    const { rerender } = open(tool());
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    rerender(
      <QueryClientProvider client={client}>
        <ClassifyDialog tool={tool({ toolId: "crm.lookup", effect: "unknown" })} tools={[]} onClose={vi.fn()} />
      </QueryClientProvider>,
    );
    expect(screen.getByLabelText("O que esta ferramenta faz com o mundo")).not.toHaveTextContent(/destrutiv/i);
  });
});
