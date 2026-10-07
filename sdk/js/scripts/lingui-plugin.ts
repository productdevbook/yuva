import type { BunPlugin } from "bun";
import { transform } from "@lingui/native-tools";

export const linguiMacro: BunPlugin = {
  name: "lingui-macro",
  setup(build) {
    build.onLoad({ filter: /\/src\/.*\.ts$/ }, async ({ path }) => {
      const source = await Bun.file(path).text();
      if (!source.includes("@lingui/core/macro")) return { contents: source, loader: "ts" };
      const { code } = await transform(source, path, { sourceMaps: false });
      return { contents: code, loader: "js" };
    });
  },
};
