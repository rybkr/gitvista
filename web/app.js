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

const HASH_RE = /^[0-9a-f]{40}$/i;

/** Extracts a commit hash from the URL fragment, or returns null. */
function getHashFromUrl() {
    const fragment = location.hash.slice(1);
    return HASH_RE.test(fragment) ? fragment : null;
}

document.addEventListener("DOMContentLoaded", () => {
    logger.info("Bootstrapping frontend");

    const root = document.querySelector("#root");
    if (!root) {
        logger.error("Root element not found");
        return;
    }

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

    const sidebar = createSidebar();
    root.parentElement.insertBefore(sidebar.el, root);
    root.appendChild(sidebar.expandBtn);

    const infoBar = createInfoBar();
    const indexView = createIndexView();
    const fileExplorer = createFileExplorer();

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
        onCommitSelect: (hash) => {
            if (hash) {
                history.replaceState(null, "", "#" + hash);
            } else {
                history.replaceState(null, "", location.pathname);
            }
        },
    });

    const keyboardHelp = createKeyboardHelp();

    createKeyboardShortcuts({
        onJumpToHead: () => graph.centerOnCommit(graph.getHeadHash()),
        onFocusSearch: () => {
            const filterInput = document.querySelector(".file-explorer-filter input");
            if (filterInput) {
                filterInput.focus();
                filterInput.select();
            }
        },
        onToggleHelp: () => keyboardHelp.toggle(),
        onDismiss: () => keyboardHelp.hide(),
        onNavigateNext: () => graph.navigateCommits("next"),
        onNavigatePrev: () => graph.navigateCommits("prev"),
    });

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
