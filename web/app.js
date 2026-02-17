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
            // Only update file explorer if it's already the active tab
            if (tabs.getActiveTab() === "file-explorer") {
                fileExplorer.openCommit(commit);
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
            fileExplorer.updateWorkingTreeStatus(status);
        },
    }).catch((error) => {
        logger.error("Backend bootstrap failed", error);
    });
});
