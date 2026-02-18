/**
 * @fileoverview Commit search component for GitVista.
 * Renders a debounced text input that filters visible commits in the graph
 * via opacity-based dimming. Search is performed across four commit fields:
 * message, author name, author email, and the 7-character hash prefix.
 *
 * The component is intentionally stateless with respect to commits — the
 * caller supplies a getCommits() accessor so the Search instance never
 * holds a stale reference to the commit Map.
 */

// Debounce delay in milliseconds — balances responsiveness and CPU cost
// when the user types quickly into the search field.
const DEBOUNCE_MS = 300;

/**
 * Creates and mounts the commit search bar into the given container element.
 *
 * @param {HTMLElement} container Element to append the search markup into.
 * @param {{
 *   getCommits: () => Map<string, import("./graph/types.js").GraphCommit>,
 *   onSearch: (result: { query: string, matchingHashes: Set<string> | null }) => void,
 * }} options
 *   - getCommits: returns the live commits Map from graph state (called on each search)
 *   - onSearch: invoked after debounce with matching commit hash Set, or null when cleared
 * @returns {{
 *   focus(): void,
 *   getValue(): string,
 *   clear(): void,
 *   destroy(): void,
 * }} Public API for the search component.
 */
export function createSearch(container, { getCommits, onSearch }) {
    // ── DOM construction ────────────────────────────────────────────────────

    const wrapper = document.createElement("div");
    wrapper.className = "commit-search";

    // Magnifier icon — purely decorative, pointer-events disabled in CSS.
    const iconEl = document.createElement("span");
    iconEl.className = "commit-search-icon";
    iconEl.setAttribute("aria-hidden", "true");
    // SVG magnifier drawn inline to avoid an extra network request.
    iconEl.innerHTML = `<svg width="13" height="13" viewBox="0 0 14 14" fill="none"
        xmlns="http://www.w3.org/2000/svg" style="display:block">
      <circle cx="5.5" cy="5.5" r="4.5" stroke="currentColor" stroke-width="1.5"/>
      <line x1="9" y1="9" x2="13" y2="13" stroke="currentColor"
            stroke-width="1.5" stroke-linecap="round"/>
    </svg>`;

    const input = document.createElement("input");
    input.type = "search";
    input.className = "commit-search-input";
    input.placeholder = "Search commits…";
    input.setAttribute("aria-label", "Search commits by message, author, or hash");
    input.setAttribute("autocomplete", "off");
    input.setAttribute("spellcheck", "false");

    // Clear button — visible only when the field is non-empty.
    const clearBtn = document.createElement("button");
    clearBtn.type = "button";
    clearBtn.className = "commit-search-clear";
    clearBtn.setAttribute("aria-label", "Clear search");
    clearBtn.textContent = "×";
    clearBtn.style.display = "none";

    wrapper.appendChild(iconEl);
    wrapper.appendChild(input);
    wrapper.appendChild(clearBtn);
    container.appendChild(wrapper);

    // ── Internal state ──────────────────────────────────────────────────────

    let debounceTimer = null;

    // ── Search logic ────────────────────────────────────────────────────────

    /**
     * Runs the search against all commits in state and notifies the caller.
     * Returns null for matchingHashes when the query is empty, which signals
     * the controller to disable all dimming (show everything).
     *
     * @param {string} rawQuery
     */
    function executeSearch(rawQuery) {
        const query = rawQuery.trim();

        if (!query) {
            onSearch({ query: "", matchingHashes: null });
            return;
        }

        const q = query.toLowerCase();
        const commits = getCommits();
        const matchingHashes = new Set();

        for (const commit of commits.values()) {
            // Search across all four fields defined by the A2 spec.
            const messageMatch = commit.message
                ? commit.message.toLowerCase().includes(q)
                : false;

            const authorName = commit.author?.name ?? commit.author?.Name ?? "";
            const nameMatch = authorName.toLowerCase().includes(q);

            const authorEmail = commit.author?.email ?? commit.author?.Email ?? "";
            const emailMatch = authorEmail.toLowerCase().includes(q);

            // Only the first 7 characters of the hash are checked — this is
            // the canonical short-hash length Git uses by default.
            const hashMatch = commit.hash
                ? commit.hash.substring(0, 7).toLowerCase().includes(q)
                : false;

            if (messageMatch || nameMatch || emailMatch || hashMatch) {
                matchingHashes.add(commit.hash);
            }
        }

        onSearch({ query, matchingHashes });
    }

    // ── Event handlers ──────────────────────────────────────────────────────

    function onInput() {
        const hasValue = input.value.length > 0;
        clearBtn.style.display = hasValue ? "flex" : "none";

        // Cancel any pending debounce and schedule a fresh one.
        clearTimeout(debounceTimer);
        debounceTimer = setTimeout(() => {
            executeSearch(input.value);
        }, DEBOUNCE_MS);
    }

    function onClear() {
        input.value = "";
        clearBtn.style.display = "none";
        clearTimeout(debounceTimer);
        onSearch({ query: "", matchingHashes: null });
        input.focus();
    }

    // Prevent the keydown event from propagating to the global shortcut handler
    // when the user is typing into the search field (e.g., pressing "/" would
    // re-focus the field while already focused — unhelpful).
    function onKeyDown(event) {
        if (event.key === "Escape") {
            // Let Escape bubble so the global Escape handler can dismiss overlays,
            // but also blur the search input to give back keyboard shortcuts.
            input.blur();
            // Do NOT stop propagation: the global handler's Escape fires onDismiss.
        }
        // All other keys are consumed naturally by the input without interference.
    }

    input.addEventListener("input", onInput);
    input.addEventListener("keydown", onKeyDown);
    clearBtn.addEventListener("click", onClear);

    // ── Public API ──────────────────────────────────────────────────────────

    return {
        /** Focuses the search input and selects any existing text. */
        focus() {
            input.focus();
            input.select();
        },

        /** Returns the current raw value of the search field. */
        getValue() {
            return input.value;
        },

        /** Clears the search field and fires onSearch with null matchingHashes. */
        clear() {
            onClear();
        },

        /** Removes DOM nodes and event listeners. */
        destroy() {
            clearTimeout(debounceTimer);
            input.removeEventListener("input", onInput);
            input.removeEventListener("keydown", onKeyDown);
            clearBtn.removeEventListener("click", onClear);
            wrapper.remove();
        },
    };
}
