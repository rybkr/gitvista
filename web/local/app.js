import { logger } from "../logger.js";
import { initThemeToggle } from "../themeToggle.js";
import { clearRoot, cleanupActiveView, bootstrapGraph } from "../gitvista/app.js";
import { parseHashFragment, parseLaunchTarget } from "../gitvista/routes.js";

let activeViewCleanup = null;

document.addEventListener("DOMContentLoaded", () => {
    logger.info("Bootstrapping frontend");
    initThemeToggle();

    const root = document.querySelector("#root");
    if (!root) {
        logger.error("Root element not found");
        return;
    }

    cleanupActiveView(activeViewCleanup);
    clearRoot(root);
    activeViewCleanup = bootstrapGraph(root, {
        parsePermalinkHash: () => parseHashFragment(location.hash).commitHash,
        parseLaunchTarget: () => parseLaunchTarget(location.search, location.hash),
        productName: "GitVista",
    });
});
