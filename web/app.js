import { logger } from "./logger.js";
import { createGraph } from "./graph.js";
import { startBackend } from "./backend.js";
import { createSidebar } from "./sidebar.js";
import { createInfoBar } from "./infoBar.js";
import { createIndexView } from "./indexView.js";
import { createSidebarTabs } from "./sidebarTabs.js";
import { createFileExplorer } from "./fileExplorer.js";
import { showToast } from "./toast.js";
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

    // Connection status dot — fixed bottom-right corner
    const statusDot = document.createElement("div");
    statusDot.title = "Connecting...";
    statusDot.style.cssText = `
        position: fixed; bottom: 16px; right: 16px;
        width: 10px; height: 10px; border-radius: 50%;
        background: #8b949e; z-index: 9999; transition: background 300ms ease;
    `;
    document.body.appendChild(statusDot);

    const styleEl = document.createElement("style");
    styleEl.textContent = `
        @keyframes _gv-pulse {
            0%, 100% { box-shadow: 0 0 0 0 rgba(26,127,55,0.5); }
            50%       { box-shadow: 0 0 0 5px rgba(26,127,55,0); }
        }
        @keyframes _gv-amber-pulse {
            0%, 100% { box-shadow: 0 0 0 0 rgba(154,103,0,0.5); }
            50%       { box-shadow: 0 0 0 5px rgba(154,103,0,0); }
        }
    `;
    document.head.appendChild(styleEl);

    function setConnectionState(state) {
        if (state === "connected") {
            statusDot.style.background = "#1a7f37";
            statusDot.style.animation = "_gv-pulse 2s ease infinite";
            statusDot.title = "Connected";
        } else if (state === "reconnecting") {
            statusDot.style.background = "#9a6700";
            statusDot.style.animation = "_gv-amber-pulse 1s ease infinite";
            statusDot.title = "Reconnecting...";
        } else {
            statusDot.style.background = "#d1242f";
            statusDot.style.animation = "none";
            statusDot.title = "Disconnected";
        }
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

    // Track branch/repo name for toast messages and document title
    let currentBranchName = "";
    let repoName = "";

    function updateTitle() {
        if (currentBranchName && repoName) {
            document.title = `${currentBranchName} — ${repoName} — GitVista`;
        } else if (repoName) {
            document.title = `${repoName} — GitVista`;
        } else {
            document.title = "GitVista";
        }
    }

    startBackend({
        logger,
        onConnectionStateChange: setConnectionState,
        onDelta: (delta) => {
            graph.applyDelta(delta);

            // Toast when new commits arrive (skip the initial bulk load by
            // gating on having a known branch name).
            const addedCount = delta.addedCommits?.length ?? 0;
            if (addedCount > 0 && currentBranchName) {
                const branchName = (delta.addedBranches && Object.keys(delta.addedBranches).length > 0)
                    ? Object.keys(delta.addedBranches)[0]
                    : currentBranchName;
                const label = addedCount === 1
                    ? `1 new commit on ${branchName}`
                    : `${addedCount} new commits on ${branchName}`;
                showToast(label, { duration: 5000 });
            }

            // Permalink restore — after the first delta populates the graph,
            // check whether the URL contains a commit hash and navigate to it.
            if (!initialDeltaApplied) {
                initialDeltaApplied = true;
                const permalinkHash = getHashFromUrl();
                if (permalinkHash) {
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
            if (headInfo.branchName) {
                currentBranchName = headInfo.branchName;
                updateTitle();
            }
            // Keep the graph controller's HEAD hash in sync so the G→H shortcut
            // and the HEAD button always navigate to the correct commit.
            graph.setHeadHash(headInfo?.hash ?? null);
        },
        onRepoMetadata: (metadata) => {
            infoBar.update(metadata);
            repoName = metadata.name || "";
            if (metadata.currentBranch) {
                currentBranchName = metadata.currentBranch;
            }
            updateTitle();
        },
    }).catch((error) => {
        logger.error("Backend bootstrap failed", error);
    });
});
