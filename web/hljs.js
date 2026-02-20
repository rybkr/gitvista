/**
 * Shared highlight.js loader and theme manager.
 *
 * Exports:
 *   loadHighlightJs()     — returns Promise<hljs | null>, cached after first call
 *   getLanguageFromPath() — maps a file path to an hljs language name, or null
 *
 * Theme management:
 *   Both the github-dark and github (light) stylesheets are injected into
 *   <head> at load time.  Their `media` attributes are flipped in response to
 *   the three-state theme toggle (system / light / dark) so exactly one
 *   stylesheet is active at any given moment without triggering a re-download.
 *
 *   A MutationObserver on document.documentElement watches for changes to the
 *   data-theme attribute, which is set by themeToggle.js.
 */

const HLJS_CSS_DARK  = "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/github-dark.min.css";
const HLJS_CSS_LIGHT = "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/github.min.css";
const HLJS_JS        = "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/highlight.min.js";

// Stylesheet elements created once and reused across theme switches
let darkLink  = null;
let lightLink = null;

// Singleton promise — resolved with the hljs object (or null on failure)
let hljsPromise = null;

/**
 * Compute and apply the correct `media` attribute to both stylesheets
 * based on the current data-theme value on <html>.
 *
 * "all"      — stylesheet is active unconditionally
 * "not all"  — stylesheet is disabled (no matching media)
 * "(prefers-color-scheme: dark/light)" — follows OS preference
 */
function syncHljsTheme() {
    if (!darkLink || !lightLink) return;

    const theme = document.documentElement.getAttribute("data-theme");

    if (theme === "dark") {
        darkLink.media  = "all";
        lightLink.media = "not all";
    } else if (theme === "light") {
        darkLink.media  = "not all";
        lightLink.media = "all";
    } else {
        // No explicit override — follow OS color scheme preference
        darkLink.media  = "(prefers-color-scheme: dark)";
        lightLink.media = "(prefers-color-scheme: light)";
    }
}

/**
 * Inject both hljs theme stylesheets into <head> and set up the
 * MutationObserver that keeps them in sync with the app's theme toggle.
 * Safe to call multiple times — idempotent.
 */
function ensureStylesheets() {
    // Avoid double-injection across multiple module evaluations (e.g. HMR)
    if (darkLink && lightLink) return;

    // Reuse existing link elements if a previous page load left them in the DOM
    darkLink  = document.querySelector(`link[data-hljs-theme="dark"]`);
    lightLink = document.querySelector(`link[data-hljs-theme="light"]`);

    if (!darkLink) {
        darkLink = document.createElement("link");
        darkLink.rel = "stylesheet";
        darkLink.href = HLJS_CSS_DARK;
        darkLink.setAttribute("data-hljs-theme", "dark");
        document.head.appendChild(darkLink);
    }

    if (!lightLink) {
        lightLink = document.createElement("link");
        lightLink.rel = "stylesheet";
        lightLink.href = HLJS_CSS_LIGHT;
        lightLink.setAttribute("data-hljs-theme", "light");
        document.head.appendChild(lightLink);
    }

    // Apply the correct initial media values
    syncHljsTheme();

    // Watch for theme changes driven by themeToggle.js
    const observer = new MutationObserver(() => syncHljsTheme());
    observer.observe(document.documentElement, {
        attributes: true,
        attributeFilter: ["data-theme"],
    });
}

/**
 * Load highlight.js from CDN (once) and inject both theme stylesheets.
 * Returns a Promise that resolves with the hljs object, or null if the
 * CDN script fails to load.
 *
 * @returns {Promise<object|null>}
 */
export function loadHighlightJs() {
    if (hljsPromise) return hljsPromise;

    hljsPromise = new Promise((resolve) => {
        ensureStylesheets();

        // Already available — a previous <script> tag loaded it
        if (window.hljs) {
            resolve(window.hljs);
            return;
        }

        const script = document.createElement("script");
        script.src = HLJS_JS;
        script.onload  = () => resolve(window.hljs ?? null);
        script.onerror = () => resolve(null); // Fail gracefully; callers render plain text
        document.head.appendChild(script);
    });

    return hljsPromise;
}

/**
 * Map a file path to an hljs language identifier.
 *
 * The mapping covers the most common languages encountered in a Git repo.
 * hljs language names must exactly match what hljs.getLanguage() accepts;
 * unknown extensions return null so the caller can fall back to plain text.
 *
 * @param {string} filePath - Full or partial file path, e.g. "src/main.go"
 * @returns {string|null} hljs language name, or null if unrecognised
 */
export function getLanguageFromPath(filePath) {
    if (!filePath) return null;

    // Extract just the basename to handle paths like "internal/server/handler.go"
    const base = filePath.split("/").pop();
    if (!base) return null;

    // Special filenames with no extension
    const FILENAME_MAP = {
        "Makefile":    "makefile",
        "makefile":    "makefile",
        "GNUmakefile": "makefile",
        "Dockerfile":  "dockerfile",
        "dockerfile":  "dockerfile",
        ".gitconfig":  "ini",
        ".gitignore":  "plaintext",
        ".env":        "shell",
        ".bashrc":     "bash",
        ".zshrc":      "bash",
        ".profile":    "bash",
    };

    if (FILENAME_MAP[base]) return FILENAME_MAP[base];

    // Extension-based lookup
    const dot = base.lastIndexOf(".");
    if (dot === -1) return null;
    const ext = base.slice(dot + 1).toLowerCase();

    const EXT_MAP = {
        // Systems / compiled
        "go":    "go",
        "rs":    "rust",
        "c":     "c",
        "h":     "c",
        "cc":    "cpp",
        "cpp":   "cpp",
        "cxx":   "cpp",
        "hpp":   "cpp",
        "cs":    "csharp",
        "java":  "java",
        "kt":    "kotlin",
        "kts":   "kotlin",
        "swift": "swift",
        "m":     "objectivec",
        "zig":   "zig",

        // Scripting / dynamic
        "js":    "javascript",
        "mjs":   "javascript",
        "cjs":   "javascript",
        "jsx":   "javascript",
        "ts":    "typescript",
        "tsx":   "typescript",
        "py":    "python",
        "pyw":   "python",
        "rb":    "ruby",
        "php":   "php",
        "lua":   "lua",
        "pl":    "perl",
        "pm":    "perl",
        "r":     "r",
        "jl":    "julia",
        "ex":    "elixir",
        "exs":   "elixir",
        "erl":   "erlang",
        "hrl":   "erlang",
        "clj":   "clojure",
        "cljc":  "clojure",
        "hs":    "haskell",
        "lhs":   "haskell",
        "ml":    "ocaml",
        "mli":   "ocaml",
        "scala": "scala",

        // Web
        "html":  "html",
        "htm":   "html",
        "css":   "css",
        "scss":  "scss",
        "sass":  "scss",
        "less":  "less",
        "svg":   "xml",

        // Data / config
        "json":  "json",
        "json5": "json",
        "yaml":  "yaml",
        "yml":   "yaml",
        "toml":  "toml",
        "xml":   "xml",
        "ini":   "ini",
        "cfg":   "ini",
        "conf":  "ini",
        "env":   "shell",

        // Shell / scripting
        "sh":    "bash",
        "bash":  "bash",
        "zsh":   "bash",
        "fish":  "bash",
        "ps1":   "powershell",
        "psm1":  "powershell",

        // Docs / markup
        "md":    "markdown",
        "mdx":   "markdown",
        "rst":   "plaintext",
        "tex":   "latex",

        // SQL / data
        "sql":   "sql",
        "graphql": "graphql",
        "gql":   "graphql",

        // Build / infra
        "tf":    "hcl",
        "hcl":   "hcl",
        "proto":  "protobuf",
        "cmake": "cmake",
        "gradle": "groovy",
        "groovy": "groovy",
    };

    return EXT_MAP[ext] ?? null;
}
