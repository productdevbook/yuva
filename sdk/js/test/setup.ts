import { GlobalRegistrator } from "@happy-dom/global-registrator";
import { linguiMacro } from "../scripts/lingui-plugin";

GlobalRegistrator.register();
Bun.plugin(linguiMacro);
