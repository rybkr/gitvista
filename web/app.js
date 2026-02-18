import { logger } from "./logger.js";
import { createGraph } from "./graph.js";
import { startBackend } from "./backend.js";
import { createSidebar } from "./sidebar.js";
import { createInfoBar } from "./infoBar.js";
import { createIndexView } from "./indexView.js";
import { createSidebarTabs } from "./sidebarTabs.js";
import { createFileExplorer } from "./fileExplorer.js";
import { createKeyboardShortcuts } from "./keyboardShortcuts.js";
import { createKeyboardHelp } from "./keyboardHelp.js";

// A valid Git commit hash is exactly 40 hex characters.
const HASH_RE = /^[0-9a-f]{40}$/i;

/**
 * Reads the current URL fragment and returns a commit hash when one is
 * present, or null when the fragment is absent or not a valid hash.
 *
 * @returns {string | null}
 */
function getHashFromUrl() {
    const fragment = location.hash.slice(1); // strip the leading #
    return HASH_RE.test(fragment) ? fragment : null;
}

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

    const infoBar = createInfoBar();
    const indexView = createIndexView();
    const fileExplorer = createFileExplorer();

    // Create wrapper for repository tab (info bar + working tree status)
    const repoTabContent = document.createElement("div");
    repoTabContent.style.display = "flex";
    repoTabContent.style.flexDirection = "column";
    repoTabContent.style.flex = "1";
    repoTabContent.style.overflow = "hidden";
    repoTabContent.appendChild(infoBar.el);
    repoTabContent.appendChild(indexView.el);

    const tabs = createSidebarTabs([
        { name: "repository", label: "Repository", content: repoTabContent },
        { name: "file-explorer", label: "File Explorer", content: fileExplorer.el },
    ]);
    sidebar.content.appendChild(tabs.el);

    // Track whether the initial delta has been applied at least once so the
    // permalink restore only fires after the graph has commits to navigate to.
    let initialDeltaApplied = false;

    const graph = createGraph(root, {
        onCommitTreeClick: (commit) => {
            // Only update file explorer if it's already the active tab
            if (tabs.getActiveTab() === "file-explorer") {
                fileExplorer.openCommit(commit);
            }
        },
        // Feature 3: Permalink — mirror the selected commit hash into the URL fragment.
        onCommitSelect: (hash) => {
            if (hash) {
                history.replaceState(null, "", "#" + hash);
            } else {
                // Selection cleared: remove the fragment without adding a history entry.
                history.replaceState(null, "", location.pathname);
            }
        },
    });

    // Feature 1b: Create the keyboard help overlay (hidden by default).
    const keyboardHelp = createKeyboardHelp();

    // Feature 1a: Register all keyboard shortcuts and wire them to graph actions.
    createKeyboardShortcuts({
        // G→H: center the viewport on the current HEAD commit.
        onJumpToHead: () => {
            graph.centerOnCommit(graph.getHeadHash());
        },
        // /: focus the file-explorer filter input when it is visible.
        onFocusSearch: () => {
            const filterInput = document.querySelector(".file-explorer-filter input");
            if (filterInput) {
                filterInput.focus();
                filterInput.select();
            }
        },
        // ?: toggle the keyboard shortcut help overlay.
        onToggleHelp: () => {
            keyboardHelp.toggle();
        },
        // Escape: dismiss the help overlay (the graph handles its own tooltip dismissal).
        onDismiss: () => {
            keyboardHelp.hide();
        },
        // J: navigate to the next (newer) commit.
        onNavigateNext: () => {
            graph.navigateCommits("next");
        },
        // K: navigate to the previous (older) commit.
        onNavigatePrev: () => {
            graph.navigateCommits("prev");
        },
    });

    startBackend({
        logger,
        onDelta: (delta) => {
            graph.applyDelta(delta);

            // Feature 3: Permalink restore — after the first delta populates the graph,
            // check whether the URL contains a commit hash and navigate to it.
            if (!initialDeltaApplied) {
                initialDeltaApplied = true;
                const permalinkHash = getHashFromUrl();
                if (permalinkHash) {
                    // Short delay lets the simulation settle its initial positions so
                    // the translateTo call lands on the node after it has been placed.
                    setTimeout(() => {
                        graph.selectAndCenter(permalinkHash);
                    }, 80);
                }
            }
        },
        onStatus: (status) => {
            indexView.update(status);
            fileExplorer.updateWorkingTreeStatus(status);
        },
        onHead: (headInfo) => {
            infoBar.updateHead(headInfo);
            // Keep the graph controller's HEAD hash in sync so the G→H shortcut
            // and the HEAD button always navigate to the correct commit.
            graph.setHeadHash(headInfo?.hash ?? null);
        },
        onRepoMetadata: (metadata) => {
            infoBar.update(metadata);
        },
    }).catch((error) => {
        logger.error("Backend bootstrap failed", error);
    });
});
