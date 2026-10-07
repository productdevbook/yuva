import { rm } from "node:fs/promises";
import { linguiMacro } from "./lingui-plugin";

process.env.NODE_ENV = "production";
await rm("dist", { recursive: true, force: true });

const builds = [
  { entry: "src/yuva.ts", format: "iife" },
  { entry: "src/yuva-chat.ts", format: "esm" },
] as const;

for (const { entry, format } of builds) {
  const result = await Bun.build({
    entrypoints: [entry],
    outdir: "dist",
    format,
    target: "browser",
    minify: true,
    define: { "process.env.NODE_ENV": '"production"' },
    plugins: [linguiMacro],
  });
  if (!result.success) {
    for (const log of result.logs) console.error(log);
    process.exit(1);
  }
}

for (const name of ["yuva.js", "yuva-chat.js"]) {
  const bytes = await Bun.file(`dist/${name}`).bytes();
  const gzip = Bun.gzipSync(bytes, { level: 9 }).length;
  console.log(`dist/${name}  ${bytes.length} B  gzip ${gzip} B`);
}
