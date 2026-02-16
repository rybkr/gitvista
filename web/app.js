import { logger } from "./logger.js";
import { createGraph } from "./graph.js";
import { startBackend } from "./backend.js";
import { createSidebar } from "./sidebar.js";
import { createIndexView } from "./indexView.js";
import { createFileSidebar } from "./fileSidebar.js";
import { createFileBrowser } from "./fileBrowser.js";

document.addEventListener("DOMContentLoaded", () => {
    logger.info("Bootstrapping frontend");

    const root = document.querySelector("#root");
    if (!root) {
        logger.error("Root element not found");
        return;
    }

    // Left sidebar (working tree status)
    const sidebar = createSidebar();
    root.parentElement.insertBefore(sidebar.el, root);
    root.appendChild(sidebar.expandBtn);

    const indexView = createIndexView();
    sidebar.content.appendChild(indexView.el);

    // Right sidebar (file browser)
    const fileSidebar = createFileSidebar();
    const fileBrowser = createFileBrowser();
    fileSidebar.contentEl.appendChild(fileBrowser.el);
    root.parentElement.appendChild(fileSidebar.el);

    const graph = createGraph(root, {
        onCommitTreeClick: (commit) => {
            fileBrowser.openCommit(commit);
            fileSidebar.expand();
        },
    });

    startBackend({
        logger,
        onDelta: (delta) => {
            graph.applyDelta(delta);
        },
        onStatus: (status) => {
            indexView.update(status);
        },
    }).catch((error) => {
        logger.error("Backend bootstrap failed", error);
    });
});
