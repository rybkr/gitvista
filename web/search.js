/**
 * @fileoverview Enhanced commit search component for GitVista.
 *
 * Renders a debounced search input with:
 *   - Structured query parsing via searchQuery.js (qualifiers: author:, hash:,
 *     after:, before:, merge:, branch:)
 *   - Inline "N / M" result count badge updated after each search
 *   - Search dropdown: qualifier suggestions + recent searches (localStorage)
 *   - Input expands on focus (CSS transition via .is-focused class)
 *   - Click-outside / Escape dismisses the dropdown
 *
 * The component is intentionally stateless with respect to graph data — the
 * caller supplies getCommits() and getCommitCount() accessors so the Search
 * instance never holds a stale reference to graph state.
 */

import { parseSearchQuery, createSearchMatcher } from "./searchQuery.js";

// Debounce delay balances responsiveness with CPU cost during fast typing.
const DEBOUNCE_MS = 200;

// Maximum recent searches stored in localStorage.
const MAX_RECENT_SEARCHES = 5;

// localStorage key for recent searches.
const RECENT_SEARCHES_KEY = "gitvista-recent-searches";

// ── Qualifier definitions ──────────────────────────────────────────────────────

/**
 * Ordered list of qualifier suggestions shown in the dropdown.
 * Each entry has the text inserted into the input and a description shown
 * alongside it.
 */
const QUALIFIERS = [
    { text: "author:", description: "Search by author name or email" },
    { text: "after:",  description: "Commits after date (e.g. 7d, 2w, 2024-01-01)" },
    { text: "before:", description: "Commits before date" },
    { text: "hash:",   description: "Search by commit hash" },
    { text: "merge:only",    description: "Show only merge commits" },
    { text: "merge:exclude", description: "Exclude merge commits" },
    { text: "branch:", description: "Commits reachable from branch" },
];

// ── Recent searches helpers ────────────────────────────────────────────────────

function loadRecentSearches() {
    try {
        const raw = localStorage.getItem(RECENT_SEARCHES_KEY);
        if (!raw) return [];
        const parsed = JSON.parse(raw);
        return Array.isArray(parsed) ? parsed.slice(0, MAX_RECENT_SEARCHES) : [];
    } catch {
        return [];
    }
}

function saveRecentSearch(query) {
    if (!query || !query.trim()) return;
    try {
        const existing = loadRecentSearches();
        // Deduplicate: move to front if already present.
        const deduped = [query, ...existing.filter((q) => q !== query)];
        localStorage.setItem(
            RECENT_SEARCHES_KEY,
            JSON.stringify(deduped.slice(0, MAX_RECENT_SEARCHES)),
        );
    } catch {
        // Ignore write errors (private browsing, quota exceeded).
    }
}

// ── Component factory ──────────────────────────────────────────────────────────

/**
 * Creates and mounts the enhanced commit search bar into the given container.
 *
 * @param {HTMLElement} container Element to append the search markup into.
 * @param {{
 *   getBranches: () => Map<string, string>,
 *   getCommits: () => Map<string, import("./graph/types.js").GraphCommit>,
 *   getCommitCount: () => { matching: number, total: number },
 *   onSearch: (result: {
 *     searchState: { query: import("./searchQuery.js").SearchQuery, matcher: ((commit: any) => boolean) | null } | null
 *   }) => void,
 * }} options
 * @returns {{
 *   focus(): void,
 *   getValue(): string,
 *   clear(): void,
 *   destroy(): void,
 * }}
 */
export function createSearch(container, { getBranches, getCommits, getCommitCount, onSearch }) {
    // ── DOM construction ───────────────────────────────────────────────────────

    // Outer positioning wrapper — provides the relative context for the dropdown.
    const positionWrapper = document.createElement("div");
    positionWrapper.className = "commit-search-positioner";

    const wrapper = document.createElement("div");
    wrapper.className = "commit-search";

    // Magnifier icon — purely decorative, pointer-events disabled in CSS.
    const iconEl = document.createElement("span");
    iconEl.className = "commit-search-icon";
    iconEl.setAttribute("aria-hidden", "true");
    iconEl.innerHTML = `<svg width="13" height="13" viewBox="0 0 14 14" fill="none"
        xmlns="http://www.w3.org/2000/svg" style="display:block">
      <circle cx="5.5" cy="5.5" r="4.5" stroke="currentColor" stroke-width="1.5"/>
      <line x1="9" y1="9" x2="13" y2="13" stroke="currentColor"
            stroke-width="1.5" stroke-linecap="round"/>
    </svg>`;

    const input = document.createElement("input");
    input.type = "search";
    input.className = "commit-search-input";
    input.placeholder = "Search commits… (e.g. author:name after:7d)";
    input.setAttribute("aria-label", "Search commits by message, author, hash, or qualifier");
    input.setAttribute("autocomplete", "off");
    input.setAttribute("spellcheck", "false");
    input.setAttribute("aria-autocomplete", "list");
    input.setAttribute("aria-haspopup", "listbox");

    // Result count badge — shown between input and clear button when a query is active.
    const resultCount = document.createElement("span");
    resultCount.className = "commit-search-result-count";
    resultCount.setAttribute("aria-live", "polite");
    resultCount.setAttribute("aria-atomic", "true");
    resultCount.style.display = "none";

    // Clear (×) button — visible only when the field is non-empty.
    const clearBtn = document.createElement("button");
    clearBtn.type = "button";
    clearBtn.className = "commit-search-clear";
    clearBtn.setAttribute("aria-label", "Clear search");
    clearBtn.textContent = "×";
    clearBtn.style.display = "none";

    wrapper.appendChild(iconEl);
    wrapper.appendChild(input);
    wrapper.appendChild(resultCount);
    wrapper.appendChild(clearBtn);

    // ── Dropdown panel ─────────────────────────────────────────────────────────

    const dropdown = document.createElement("div");
    dropdown.className = "commit-search-dropdown";
    dropdown.setAttribute("role", "listbox");
    dropdown.setAttribute("aria-label", "Search suggestions");
    dropdown.style.display = "none";

    positionWrapper.appendChild(wrapper);
    positionWrapper.appendChild(dropdown);
    container.appendChild(positionWrapper);

    // ── Internal state ─────────────────────────────────────────────────────────

    let debounceTimer = null;
    let isDropdownOpen = false;

    // ── Dropdown rendering ─────────────────────────────────────────────────────

    /**
     * Renders the dropdown contents based on the current input value.
     * Shows qualifier suggestions and recent searches.
     */
    function renderDropdown() {
        dropdown.innerHTML = "";
        const recent = loadRecentSearches();

        // Qualifier suggestions section
        const qualifierSection = document.createElement("div");
        qualifierSection.className = "commit-search-dropdown-section";

        const qualifierHeading = document.createElement("div");
        qualifierHeading.className = "commit-search-dropdown-heading";
        qualifierHeading.textContent = "Qualifiers";
        qualifierSection.appendChild(qualifierHeading);

        for (const q of QUALIFIERS) {
            const item = document.createElement("div");
            item.className = "commit-search-suggestion";
            item.setAttribute("role", "option");
            item.setAttribute("tabindex", "-1");

            const kw = document.createElement("span");
            kw.className = "commit-search-suggestion-keyword";
            kw.textContent = q.text;

            const desc = document.createElement("span");
            desc.className = "commit-search-suggestion-desc";
            desc.textContent = q.description;

            item.appendChild(kw);
            item.appendChild(desc);

            // Clicking a suggestion appends the qualifier text to the current input value.
            item.addEventListener("mousedown", (e) => {
                e.preventDefault(); // Prevent blur on input
                insertQualifier(q.text);
            });

            qualifierSection.appendChild(item);
        }

        dropdown.appendChild(qualifierSection);

        // Recent searches section — only shown when there are any
        if (recent.length > 0) {
            const recentSection = document.createElement("div");
            recentSection.className = "commit-search-dropdown-section";

            const recentHeading = document.createElement("div");
            recentHeading.className = "commit-search-dropdown-heading";
            recentHeading.textContent = "Recent";
            recentSection.appendChild(recentHeading);

            for (const recentQuery of recent) {
                const item = document.createElement("div");
                item.className = "commit-search-recent";
                item.setAttribute("role", "option");
                item.setAttribute("tabindex", "-1");

                // Clock icon
                const clockIcon = document.createElement("span");
                clockIcon.className = "commit-search-recent-icon";
                clockIcon.setAttribute("aria-hidden", "true");
                clockIcon.innerHTML = `<svg width="12" height="12" viewBox="0 0 12 12" fill="none"
                    xmlns="http://www.w3.org/2000/svg">
                  <circle cx="6" cy="6" r="5" stroke="currentColor" stroke-width="1.2"/>
                  <polyline points="6,3 6,6 8,7.5" stroke="currentColor" stroke-width="1.2"
                    stroke-linecap="round" stroke-linejoin="round"/>
                </svg>`;

                const text = document.createElement("span");
                text.className = "commit-search-recent-text";
                text.textContent = recentQuery;

                item.appendChild(clockIcon);
                item.appendChild(text);

                item.addEventListener("mousedown", (e) => {
                    e.preventDefault(); // Prevent blur
                    applyRecentSearch(recentQuery);
                });

                recentSection.appendChild(item);
            }

            dropdown.appendChild(recentSection);
        }
    }

    /** Inserts a qualifier keyword at the end of the current input value. */
    function insertQualifier(qualifierText) {
        const current = input.value;
        // If there's already content and it doesn't end with a space, add one.
        if (current && !current.endsWith(" ")) {
            input.value = current + " " + qualifierText;
        } else {
            input.value = current + qualifierText;
        }
        input.focus();
        // Position cursor at end
        const len = input.value.length;
        input.setSelectionRange(len, len);
        // Trigger search with new value
        onInputChange();
    }

    /** Applies a recent search query. */
    function applyRecentSearch(query) {
        input.value = query;
        input.focus();
        closeDropdown();
        // Run immediately without debounce for recent search clicks
        executeSearch(query);
    }

    // ── Dropdown open/close ────────────────────────────────────────────────────

    function openDropdown() {
        if (isDropdownOpen) return;
        isDropdownOpen = true;
        renderDropdown();
        dropdown.style.display = "block";
        wrapper.classList.add("is-focused");
        document.addEventListener("pointerdown", onOutsidePointerDown, true);
    }

    function closeDropdown() {
        if (!isDropdownOpen) return;
        isDropdownOpen = false;
        dropdown.style.display = "none";
        wrapper.classList.remove("is-focused");
        document.removeEventListener("pointerdown", onOutsidePointerDown, true);
    }

    /**
     * Closes the dropdown when a pointer-down event fires outside the
     * positioner wrapper (which contains both the input and dropdown).
     */
    function onOutsidePointerDown(e) {
        if (!positionWrapper.contains(e.target)) {
            closeDropdown();
        }
    }

    // ── Result count display ───────────────────────────────────────────────────

    /**
     * Updates the inline result count badge.
     * Shows "N / M" when a query is active; hides when search is cleared.
     *
     * @param {boolean} hasQuery Whether an active search is running.
     */
    function updateResultCount(hasQuery) {
        if (!hasQuery) {
            resultCount.style.display = "none";
            resultCount.textContent = "";
            return;
        }
        const { matching, total } = getCommitCount();
        resultCount.textContent = `${matching} / ${total}`;
        resultCount.style.display = "inline-flex";
        // Warning styling when zero results.
        resultCount.classList.toggle("is-empty", matching === 0);
    }

    // ── Search execution ────────────────────────────────────────────────────────

    /**
     * Parses the query, builds a matcher, fires onSearch, and updates the
     * result count badge.  Persists non-empty queries to recent searches.
     *
     * @param {string} rawQuery
     */
    function executeSearch(rawQuery) {
        const query = parseSearchQuery(rawQuery);

        if (query.isEmpty) {
            onSearch({ searchState: null });
            updateResultCount(false);
            return;
        }

        // Build the matcher with live graph data (called at search time so it
        // always captures the current branches/commits maps).
        const matcher = createSearchMatcher(query, getBranches(), getCommits());

        const searchState = { query, matcher };
        onSearch({ searchState });

        // Update result count on the next tick after the graph has applied dimming.
        // The 0ms timeout gives the graph controller time to run applyDimmingFromPredicate.
        requestAnimationFrame(() => {
            updateResultCount(true);
        });

        // Persist to recent searches if the query has meaningful content.
        if (rawQuery.trim()) {
            saveRecentSearch(rawQuery.trim());
        }
    }

    // ── Event handlers ─────────────────────────────────────────────────────────

    function onInputChange() {
        const hasValue = input.value.length > 0;
        clearBtn.style.display = hasValue ? "flex" : "none";

        if (!hasValue) {
            resultCount.style.display = "none";
        }

        clearTimeout(debounceTimer);
        debounceTimer = setTimeout(() => {
            executeSearch(input.value);
        }, DEBOUNCE_MS);
    }

    function onFocus() {
        openDropdown();
    }

    function onClear() {
        input.value = "";
        clearBtn.style.display = "none";
        resultCount.style.display = "none";
        clearTimeout(debounceTimer);
        onSearch({ searchState: null });
        closeDropdown();
        input.focus();
    }

    /**
     * Keydown handler on the search input.
     * Escape blurs/clears but lets the event bubble to global shortcuts.
     */
    function onKeyDown(event) {
        if (event.key === "Escape") {
            // Close dropdown first; if already closed, let global handler run
            if (isDropdownOpen) {
                closeDropdown();
                event.stopPropagation();
            } else {
                input.blur();
                // Do NOT stopPropagation: let the global Escape handler fire onDismiss.
            }
        } else if (event.key === "Enter") {
            // On Enter, persist current query to recent searches immediately.
            const val = input.value.trim();
            if (val) {
                saveRecentSearch(val);
                // Re-render dropdown with updated recents (if open)
                if (isDropdownOpen) renderDropdown();
            }
        }
        // All printable keys are handled naturally by the input.
    }

    input.addEventListener("input", onInputChange);
    input.addEventListener("focus", onFocus);
    input.addEventListener("keydown", onKeyDown);
    clearBtn.addEventListener("click", onClear);

    // ── Public API ─────────────────────────────────────────────────────────────

    return {
        /** Focuses the search input and opens the dropdown. */
        focus() {
            input.focus();
            input.select();
        },

        /** Returns the current raw value of the search field. */
        getValue() {
            return input.value;
        },

        /** Clears the search field and fires onSearch with null searchState. */
        clear() {
            onClear();
        },

        /** Removes DOM nodes and event listeners. */
        destroy() {
            clearTimeout(debounceTimer);
            input.removeEventListener("input", onInputChange);
            input.removeEventListener("focus", onFocus);
            input.removeEventListener("keydown", onKeyDown);
            clearBtn.removeEventListener("click", onClear);
            document.removeEventListener("pointerdown", onOutsidePointerDown, true);
            positionWrapper.remove();
        },
    };
}
