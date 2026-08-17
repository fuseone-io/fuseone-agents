import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CredentialFields } from "@/features/integrations/mcp/credential-fields";

/*
Removing a credential is a gesture of its own.

An empty field means "leave what is stored", which is why correcting an address
does not demand re-entering a secret nobody has to hand. With only that rule a
credential could be written and never taken back — and the day that matters is
the day it leaked.
*/
describe("the credential fields", () => {
  it("offers a way to remove what is stored", async () => {
    const revoke = vi.fn();
    render(
      <CredentialFields
        local={false}
        hasSecret
        hasConfigFile={false}
        value={{ token: "", env: "", configFile: "", configFileEnv: "" }}
        onChange={vi.fn()}
        onRevoke={revoke}
        onRevokeConfigFile={vi.fn()}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: /remover/i }));
    expect(revoke).toHaveBeenCalledOnce();
  });

  it("offers nothing to remove when nothing is stored", () => {
    render(
      <CredentialFields
        local={false}
        hasSecret={false}
        hasConfigFile={false}
        value={{ token: "", env: "", configFile: "", configFileEnv: "" }}
        onChange={vi.fn()}
        onRevoke={vi.fn()}
        onRevokeConfigFile={vi.fn()}
      />,
    );
    expect(screen.queryByRole("button", { name: /remover/i })).not.toBeInTheDocument();
  });

  it("asks a local server for variables and a remote one for a bearer", () => {
    const { rerender } = render(
      <CredentialFields
        local
        hasSecret={false}
        hasConfigFile={false}
        value={{ token: "", env: "", configFile: "", configFileEnv: "" }}
        onChange={vi.fn()}
        onRevoke={vi.fn()}
        onRevokeConfigFile={vi.fn()}
      />,
    );
    expect(screen.getByLabelText(/variáveis/i)).toBeInTheDocument();
    expect(
      screen.getByLabelText(/^arquivo de configuração$/i),
    ).toBeInTheDocument();

    rerender(
      <CredentialFields
        local={false}
        hasSecret={false}
        hasConfigFile={false}
        value={{ token: "", env: "", configFile: "", configFileEnv: "" }}
        onChange={vi.fn()}
        onRevoke={vi.fn()}
        onRevokeConfigFile={vi.fn()}
      />,
    );
    expect(screen.queryByLabelText(/variáveis/i)).not.toBeInTheDocument();
  });

  it("removes a managed config file separately from variables", async () => {
    const revokeFile = vi.fn();
    render(
      <CredentialFields
        local
        hasSecret={false}
        hasConfigFile
        value={{ token: "", env: "", configFile: "", configFileEnv: "" }}
        onChange={vi.fn()}
        onRevoke={vi.fn()}
        onRevokeConfigFile={revokeFile}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: /arquivo de configuração/i }),
    );
    expect(revokeFile).toHaveBeenCalledOnce();
  });
});
