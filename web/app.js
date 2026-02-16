import { logger } from "./logger.js";
import { createGraph } from "./graph.js";
import { startBackend } from "./backend.js";
import { createSidebar } from "./sidebar.js";
import { createIndexView } from "./indexView.js";
import { createSidebarTabs } from "./sidebarTabs.js";
import { createFileExplorer } from "./fileExplorer.js";

document.addEventListener("DOMContentLoaded", () => {
    logger.info("Bootstrapping frontend");

    const root = document.querySelector("#root");
    if (!root) {
        logger.error("Root element not found");
        return;
    }

    // Left sidebar with tabs
    const sidebar = createSidebar();
    root.parentElement.insertBefore(sidebar.el, root);
    root.appendChild(sidebar.expandBtn);

    const indexView = createIndexView();
    const fileExplorer = createFileExplorer();

    const tabs = createSidebarTabs([
        { name: "working-tree", label: "Working Tree", content: indexView.el },
        { name: "file-explorer", label: "File Explorer", content: fileExplorer.el },
    ]);
    sidebar.content.appendChild(tabs.el);

    const graph = createGraph(root, {
        onCommitTreeClick: (commit) => {
            tabs.showTab("file-explorer");
            fileExplorer.openCommit(commit);
            // Ensure sidebar is visible
            if (sidebar.el.classList.contains("is-collapsed")) {
                sidebar.expandBtn.click();
            }
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
