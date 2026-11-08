import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Toaster } from "@/components/ui/sonner";
import { ServerEditor } from "@/features/integrations/server-editor";
import { ServerCard } from "@/features/integrations/server-card";
import { MCPServerPage } from "@/features/integrations/mcp/mcp-server-page";
import type { MCPServer } from "@/features/integrations/api";
import { setLocale } from "@/i18n";
import { json, revisedCollection, stubNetwork } from "@/test/revised-routes";

const github: MCPServer = {
  name: "github",
  transport: "http",
  url: "https://mcp.example.com/a",
  enabled: true,
};

function world(listed?: MCPServer[]) {
  const servers = revisedCollection("/admin/integrations/mcp-servers", [
    github,
  ]);
  const requests = stubNetwork([servers], (_method, path) => {
    if (path === "/admin/integrations") {
      return json({ mcpServers: listed ?? servers.state.items, providers: [] });
    }
    if (path === "/admin/tools") {
      return json({
        items: [
          { toolId: "github.search", server: "github", effect: "read", untrusted: true },
          { toolId: "github.issues", server: "github", effect: "read", untrusted: true },
        ],
      });
    }
    return undefined;
  });
  const writes = (method: string) =>
    requests.filter((r) => r.method === method);
  return { servers, requests, writes };
}

const fresh = () =>
  new QueryClient({ defaultOptions: { queries: { retry: false } } });

function show(node: ReactNode, path = "/", client = fresh()) {
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/integrations/mcp/:name" element={<MCPServerPage />} />
          <Route path="*" element={node} />
        </Routes>
      </MemoryRouter>
      <Toaster />
    </QueryClientProvider>,
  );
}

describe("a tool server written over what was read", () => {
  beforeEach(() => {
    setLocale("en-US");
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.scrollIntoView ??= () => {};
  });
  afterEach(() => vi.unstubAllGlobals());

  it("is read by name when its editor opens, and saved under that revision", async () => {
    const net = world();
    show(<ServerEditor server={github} onClose={vi.fn()} />);
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Save" }));

    await waitFor(() => expect(net.writes("PUT")).toHaveLength(1));
    expect(
      net.requests.some(
        (r) =>
          r.method === "GET" &&
          r.path === "/admin/integrations/mcp-servers/github",
      ),
    ).toBe(true);
    expect(net.writes("PUT")[0]?.ifMatch).toBe('"v1-github-1"');
  });

  it("keeps the edit when somebody wrote first, and reloads only when asked", async () => {
    const user = userEvent.setup();
    const net = world();
    show(<ServerEditor server={github} onClose={vi.fn()} />);
    const address = await screen.findByLabelText("Address");
    await user.clear(address);
    await user.type(address, "https://mcp.example.com/mine");
    net.servers.elsewhere("github", { url: "https://mcp.example.com/theirs" });
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(
      await screen.findByText("Somebody changed this while you were editing"),
    ).toBeVisible();
    expect(screen.getByLabelText("Address")).toHaveValue(
      "https://mcp.example.com/mine",
    );
    await user.click(
      screen.getByRole("button", { name: "Reload what is stored" }),
    );
    await waitFor(() =>
      expect(screen.getByLabelText("Address")).toHaveValue(
        "https://mcp.example.com/theirs",
      ),
    );
  });

  it("saves the surface under the revision its page read, and says when it went stale", async () => {
    const user = userEvent.setup();
    const net = world();
    show(null, "/integrations/mcp/github");
    await user.click(await screen.findByLabelText("github.search"));
    net.servers.elsewhere("github", { url: "https://mcp.example.com/theirs" });
    await user.click(
      screen.getByRole("button", { name: "Save what we brought in" }),
    );

    expect(
      await screen.findByText("Somebody changed this while you were editing"),
    ).toBeVisible();
    expect(net.writes("PUT")[0]?.ifMatch).toBe('"v1-github-1"');
    expect(net.servers.state.items[0]?.url).toBe(
      "https://mcp.example.com/theirs",
    );
  });

  it("is removed under the revision read when its confirmation opened", async () => {
    const user = userEvent.setup();
    const net = world();
    show(<ServerCard server={github} onEdit={vi.fn()} />);
    net.servers.elsewhere("github", {});
    await user.click(screen.getByRole("button", { name: "Remove github?" }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Remove" })).toBeEnabled(),
    );
    await user.click(screen.getByRole("button", { name: "Remove" }));

    await waitFor(() => expect(net.writes("DELETE")).toHaveLength(1));
    expect(net.writes("DELETE")[0]?.ifMatch).toBe('"v1-github-2"');
  });

  // The connection panel writes the same record, so it answers the same revision.
  it("revokes a credential under the revision its page read", async () => {
    const user = userEvent.setup();
    const net = world();
    net.servers.state.items = [{ ...github, hasSecret: true, hasOAuth: true }];
    show(null, "/integrations/mcp/github");
    const revoke = await screen.findByRole("button", {
      name: /Remove stored OAuth grant/i,
    });
    net.servers.elsewhere("github", {});
    await user.click(revoke);

    expect(
      await screen.findByText("Somebody changed this while you were editing"),
    ).toBeVisible();
    expect(net.writes("PUT")[0]?.ifMatch).toBe('"v1-github-1"');
  });

  // A listing can lag the record. The surface travels with the whole server,
  // so what goes back is what was read by name, under the revision it answers.
  it("sends back the server it read by name, not the listing", async () => {
    const user = userEvent.setup();
    const net = world([{ ...github, url: "https://mcp.example.com/listed" }]);
    show(null, "/integrations/mcp/github");
    await user.click(await screen.findByLabelText("github.search"));
    await user.click(
      screen.getByRole("button", { name: "Save what we brought in" }),
    );

    await waitFor(() => expect(net.writes("PUT")).toHaveLength(1));
    expect(net.writes("PUT")[0]?.body).toMatchObject({
      url: "https://mcp.example.com/a",
    });
  });

  // Its writes send the whole record back, so a cached one would put back
  // whatever changed since the page was last opened.
  it("sends back the server as read when its page opened, not a cached copy", async () => {
    const user = userEvent.setup();
    const net = world();
    const client = fresh();
    const first = show(null, "/integrations/mcp/github", client);
    await screen.findByLabelText("github.search");
    first.unmount();
    net.servers.elsewhere("github", { url: "https://mcp.example.com/theirs" });
    const release = net.servers.hold();
    show(null, "/integrations/mcp/github", client);

    // Nothing is offered to write while the page's own read is unanswered.
    const reads = () => net.requests.filter((r) => r.path === "/admin/integrations/mcp-servers/github");
    await waitFor(() => expect(reads()).toHaveLength(2));
    expect(screen.queryByLabelText("github.search")).toBeNull();
    release();
    await user.click(await screen.findByLabelText("github.search"));
    await user.click(screen.getByRole("button", { name: "Save what we brought in" }));

    await waitFor(() => expect(net.writes("PUT")).toHaveLength(1));
    expect(net.writes("PUT")[0]?.body).toMatchObject({ url: "https://mcp.example.com/theirs" });
  });

  // The listing still shows the surface somebody replaced; the server read by
  // name has the new one. What is shown and sent must be the one the ETag names.
  it("starts the surface from the server read by name, not the listing", async () => {
    const user = userEvent.setup();
    const net = world([{ ...github, surface: [] }]);
    net.servers.state.items = [{ ...github, surface: ["search"] }];
    show(null, "/integrations/mcp/github");

    expect(await screen.findByLabelText("github.search")).toBeChecked();
    expect(screen.getByLabelText("github.issues")).not.toBeChecked();
    await user.click(screen.getByLabelText("github.search"));
    await user.click(screen.getByRole("button", { name: "Save what we brought in" }));
    await waitFor(() => expect(net.writes("PUT")).toHaveLength(1));
    expect(net.writes("PUT")[0]?.body).toMatchObject({ surface: [] });
  });

  it("says the server could not be read, and retries that read", async () => {
    const user = userEvent.setup();
    const net = world();
    net.servers.reads.mode = "failing";
    show(null, "/integrations/mcp/github");
    const retry = await screen.findByRole("button", { name: "Try again" });
    // What failed is the read, so the server's own problem is what is said.
    expect(screen.getByText("boom")).toBeVisible();
    net.servers.reads.mode = "normal";
    await user.click(retry);

    expect(await screen.findByLabelText("github.search")).toBeVisible();
  });

  it("opens no cached copy when reading the server again fails", async () => {
    const net = world();
    const client = fresh();
    const first = show(null, "/integrations/mcp/github", client);
    await screen.findByLabelText("github.search");
    first.unmount();
    net.servers.reads.mode = "failing";
    show(null, "/integrations/mcp/github", client);

    expect(await screen.findByRole("button", { name: "Try again" })).toBeVisible();
    expect(screen.queryByLabelText("github.search")).toBeNull();
  });

  // Removed elsewhere, with the listing still behind: the read by name is the
  // answer, and the copy cached by the last opening must not stand in for it.
  it("opens no cached copy of a server that is gone, whatever the listing says", async () => {
    const net = world([github]);
    const client = fresh();
    const first = show(null, "/integrations/mcp/github", client);
    await screen.findByLabelText("github.search");
    first.unmount();
    net.servers.state.items = [];
    show(null, "/integrations/mcp/github", client);

    expect(await screen.findByRole("button", { name: "Try again" })).toBeVisible();
    expect(screen.queryByLabelText("github.search")).toBeNull();
  });
  // Saving the surface writes a new revision; a credential being typed in the
  // panel beside it is not part of that, and must still be there.
  it("keeps a credential being typed when the surface is saved", async () => {
    const user = userEvent.setup();
    world();
    show(null, "/integrations/mcp/github");
    await user.type(await screen.findByLabelText("Bearer token"), "typed-secret");
    await user.click(screen.getByLabelText("github.search"));
    await user.click(screen.getByRole("button", { name: "Save what we brought in" }));

    expect(await screen.findByText("Saved.")).toBeVisible();
    expect(screen.getByLabelText("Bearer token")).toHaveValue("typed-secret");
  });
});
