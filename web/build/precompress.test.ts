import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import zlib from "node:zlib";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { precompress } from "./precompress";

// The Go handler serves name.br or name.gz in place of name whenever they
// exist, without ever decoding them. So a variant that is not exactly the
// original compressed would be served as the console, and a variant written
// for a format that is already compressed would only cost bytes.

let dir: string;
const script = "export const x = 1;\n".repeat(200);

beforeEach(async () => {
  dir = await fs.mkdtemp(path.join(os.tmpdir(), "precompress-"));
  await fs.mkdir(path.join(dir, "assets"));
  await fs.writeFile(path.join(dir, "assets", "app.js"), script);
  await fs.writeFile(path.join(dir, "assets", "font.woff2"), "wOF2".repeat(500));
});

afterEach(() => fs.rm(dir, { recursive: true, force: true }));

describe("precompress", () => {
  it("writes brotli and gzip copies that decode to the original", async () => {
    await precompress(dir);

    const br = await fs.readFile(path.join(dir, "assets", "app.js.br"));
    const gz = await fs.readFile(path.join(dir, "assets", "app.js.gz"));
    expect(zlib.brotliDecompressSync(br).toString()).toBe(script);
    expect(zlib.gunzipSync(gz).toString()).toBe(script);
  });

  it("leaves formats that are already compressed alone", async () => {
    await precompress(dir);

    const names = await fs.readdir(path.join(dir, "assets"));
    expect(names.filter((n) => n.startsWith("font"))).toEqual(["font.woff2"]);
  });
});
