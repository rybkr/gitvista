/**
 * File type icon mapping — returns colored inline SVGs based on file extension.
 * Each icon is 14x14 with a distinctive fill color from the GitHub Primer palette.
 */

const fileIcon = (color) => `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M4 2C3.44772 2 3 2.44772 3 3V13C3 13.5523 3.44772 14 4 14H12C12.5523 14 13 13.5523 13 13V6L9 2H4Z" fill="${color}" opacity="0.2"/>
    <path d="M9 2V6H13M4 2H9L13 6V13C13 13.5523 12.5523 14 12 14H4C3.44772 14 3 13.5523 3 13V3C3 2.44772 3.44772 2 4 2Z" stroke="${color}" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;

const ICON_MAP = {
    // Go
    go: fileIcon("#00ADD8"),
    mod: fileIcon("#00ADD8"),
    sum: fileIcon("#00ADD8"),
    // JavaScript
    js: fileIcon("#f0db4f"),
    mjs: fileIcon("#f0db4f"),
    cjs: fileIcon("#f0db4f"),
    jsx: fileIcon("#f0db4f"),
    // TypeScript
    ts: fileIcon("#3178c6"),
    tsx: fileIcon("#3178c6"),
    // HTML
    html: fileIcon("#e34c26"),
    htm: fileIcon("#e34c26"),
    // CSS
    css: fileIcon("#1572b6"),
    scss: fileIcon("#c6538c"),
    less: fileIcon("#1d365d"),
    // JSON
    json: fileIcon("#9a6700"),
    // Markdown
    md: fileIcon("#0969da"),
    mdx: fileIcon("#0969da"),
    // YAML
    yml: fileIcon("#cb171e"),
    yaml: fileIcon("#cb171e"),
    // Shell
    sh: fileIcon("#1a7f37"),
    bash: fileIcon("#1a7f37"),
    zsh: fileIcon("#1a7f37"),
    fish: fileIcon("#1a7f37"),
    // Images
    png: fileIcon("#8250df"),
    jpg: fileIcon("#8250df"),
    jpeg: fileIcon("#8250df"),
    gif: fileIcon("#8250df"),
    svg: fileIcon("#8250df"),
    ico: fileIcon("#8250df"),
    webp: fileIcon("#8250df"),
    // Config
    toml: fileIcon("#656d76"),
    ini: fileIcon("#656d76"),
    cfg: fileIcon("#656d76"),
    conf: fileIcon("#656d76"),
    env: fileIcon("#656d76"),
    // Python
    py: fileIcon("#3572A5"),
    // Ruby
    rb: fileIcon("#CC342D"),
    // Rust
    rs: fileIcon("#dea584"),
    // Java
    java: fileIcon("#b07219"),
    // C/C++
    c: fileIcon("#555555"),
    h: fileIcon("#555555"),
    cpp: fileIcon("#f34b7d"),
    cc: fileIcon("#f34b7d"),
    hpp: fileIcon("#f34b7d"),
    // Lock files
    lock: fileIcon("#656d76"),
    // SQL
    sql: fileIcon("#e38c00"),
    // XML
    xml: fileIcon("#0060ac"),
};

const NAME_MAP = {
    "Makefile": fileIcon("#e37933"),
    "Dockerfile": fileIcon("#2496ed"),
    "Containerfile": fileIcon("#2496ed"),
    ".gitignore": fileIcon("#f05133"),
    ".gitmodules": fileIcon("#f05133"),
    ".gitattributes": fileIcon("#f05133"),
    ".dockerignore": fileIcon("#2496ed"),
    "LICENSE": fileIcon("#656d76"),
    "CLAUDE.md": fileIcon("#d4a574"),
    "go.mod": fileIcon("#00ADD8"),
    "go.sum": fileIcon("#00ADD8"),
    "package.json": fileIcon("#cb3837"),
    "package-lock.json": fileIcon("#656d76"),
    "tsconfig.json": fileIcon("#3178c6"),
};

const FALLBACK = fileIcon("#656d76");

/**
 * Get a colored SVG icon string for a given filename.
 * @param {string} filename
 * @returns {string} An inline SVG string
 */
export function getFileIcon(filename) {
    if (NAME_MAP[filename]) return NAME_MAP[filename];

    // Dotfile config patterns (.eslintrc, .prettierrc, etc.)
    if (filename.startsWith(".") && filename.endsWith("rc")) {
        return fileIcon("#656d76");
    }

    const dotIndex = filename.lastIndexOf(".");
    if (dotIndex > 0) {
        const ext = filename.substring(dotIndex + 1).toLowerCase();
        if (ICON_MAP[ext]) return ICON_MAP[ext];
    }

    return FALLBACK;
}
