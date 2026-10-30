import fs from "node:fs/promises";
import path from "node:path";
import { promisify } from "node:util";
import zlib from "node:zlib";
import type { Plugin } from "vite";

// The console is served by the Go binary, which has no compression of its
// own. Compressing once here, at maximum effort, costs build seconds instead
// of CPU on every request, and the handler in internal/web picks the copy the
// browser accepts. Fonts and images are already compressed and are skipped.
const COMPRESSIBLE = new Set([".js", ".css", ".html", ".svg", ".json", ".txt"]);

const brotli = promisify(zlib.brotliCompress);
const gzip = promisify(zlib.gzip);

const encoders: Array<[string, (b: Buffer) => Promise<Buffer>]> = [
  [
    ".br",
    (b) =>
      brotli(b, {
        params: {
          [zlib.constants.BROTLI_PARAM_QUALITY]: zlib.constants.BROTLI_MAX_QUALITY,
          [zlib.constants.BROTLI_PARAM_SIZE_HINT]: b.length,
        },
      }),
  ],
  [".gz", (b) => gzip(b, { level: zlib.constants.Z_BEST_COMPRESSION })],
];

/** Writes name.br and name.gz beside every compressible file under dir. */
export async function precompress(dir: string): Promise<void> {
  const entries = await fs.readdir(dir, { recursive: true, withFileTypes: true });
  const files = entries
    .filter((e) => e.isFile() && COMPRESSIBLE.has(path.extname(e.name)))
    .map((e) => path.join(e.parentPath, e.name));
  await Promise.all(files.map(compressOne));
}

async function compressOne(file: string): Promise<void> {
  const original = await fs.readFile(file);
  for (const [suffix, encode] of encoders) {
    const encoded = await encode(original);
    // A copy that is not smaller is only more bytes in the binary.
    if (encoded.length < original.length) await fs.writeFile(file + suffix, encoded);
  }
}

/** Runs precompress over the build output once Vite has written it. */
export function precompressPlugin(): Plugin {
  return {
    name: "fuseone:precompress",
    apply: "build",
    async writeBundle(options) {
      if (options.dir) await precompress(options.dir);
    },
  };
}
