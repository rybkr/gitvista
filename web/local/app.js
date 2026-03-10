import { logger } from "../logger.js";
import { initThemeToggle } from "../themeToggle.js";
import { clearRoot, cleanupActiveView, bootstrapGraph } from "../gitvista/app.js";
import { parseLocalHash, parseLocalLaunchTarget } from "../gitvista/routes.js";

let activeViewCleanup = null;

document.addEventListener("DOMContentLoaded", () => {
    logger.info("Bootstrapping local frontend");
    initThemeToggle();

    const root = document.querySelector("#root");
    if (!root) {
        logger.error("Root element not found");
        return;
    }

    cleanupActiveView(activeViewCleanup);
    clearRoot(root);
    activeViewCleanup = bootstrapGraph(root, {
        parsePermalinkHash: () => parseLocalHash(location.hash)?.commitHash || null,
        parseLaunchTarget: () => parseLocalLaunchTarget(location.search, location.hash),
        productName: "GitVista",
    });
});
