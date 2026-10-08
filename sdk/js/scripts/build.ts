import { rm } from "node:fs/promises";
import { linguiMacro } from "./lingui-plugin";
import pkg from "../package.json";

process.env.NODE_ENV = "production";
await rm("dist", { recursive: true, force: true });

const define = { "process.env.NODE_ENV": '"production"' };

function check(result: Bun.BuildOutput): void {
  if (result.success) return;
  for (const log of result.logs) console.error(log);
  process.exit(1);
}

const scripts = [
  { entry: "src/yuva.ts", format: "iife" },
  { entry: "src/yuva-chat.ts", format: "esm" },
] as const;

for (const { entry, format } of scripts) {
  check(await Bun.build({ entrypoints: [entry], outdir: "dist", format, target: "browser", minify: true, define, plugins: [linguiMacro] }));
}

const entries = ["src/index.ts", "src/chat.ts", "src/react.ts", "src/api.ts"];
check(
  await Bun.build({
    entrypoints: entries,
    outdir: "dist/esm",
    root: "src",
    format: "esm",
    target: "node",
    splitting: true,
    minify: { syntax: true, whitespace: true },
    naming: { chunk: "[name]-[hash].js" },
    external: [...Object.keys(pkg.dependencies), ...Object.keys(pkg.peerDependencies)],
    define,
    plugins: [linguiMacro],
  }),
);

const tsc = Bun.spawnSync(["bunx", "tsc", "-p", "tsconfig.build.json"], { stdout: "inherit", stderr: "inherit" });
if (tsc.exitCode !== 0) process.exit(tsc.exitCode ?? 1);

// Node's ESM resolution (moduleResolution node16/nodenext) needs explicit extensions in declarations too.
for await (const path of new Bun.Glob("dist/types/**/*.d.ts").scan()) {
  const text = await Bun.file(path).text();
  await Bun.write(path, text.replace(/((?:from|import\()\s*")(\.\.?\/[^"]+?)(?<!\.js)"/g, '$1$2.js"'));
}

for (const name of ["yuva.js", "yuva-chat.js", "esm/index.js", "esm/chat.js", "esm/react.js", "esm/api.js"]) {
  const bytes = await Bun.file(`dist/${name}`).bytes();
  const gzip = Bun.gzipSync(bytes, { level: 9 }).length;
  console.log(`dist/${name}  ${bytes.length} B  gzip ${gzip} B`);
}
