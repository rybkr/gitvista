const DEFAULT_WIDTH = 320;
const MIN_WIDTH = 200;
const COLLAPSED_WIDTH = 0;
const STORAGE_KEY = "gitvista-file-sidebar";

function loadState() {
    try {
        const raw = localStorage.getItem(STORAGE_KEY);
        if (raw) return JSON.parse(raw);
    } catch {
        // ignore
    }
    return { width: DEFAULT_WIDTH, collapsed: true };
}

function saveState(state) {
    try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
    } catch {
        // ignore
    }
}

const CLOSE_SVG = `<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
    <path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
</svg>`;

export function createFileSidebar() {
    const saved = loadState();
    let width = saved.width ?? DEFAULT_WIDTH;
    let collapsed = saved.collapsed ?? true;

    // Main sidebar element
    const el = document.createElement("aside");
    el.className = "file-sidebar";

    const header = document.createElement("div");
    header.className = "file-sidebar-header";

    const title = document.createElement("div");
    title.className = "file-sidebar-title";
    title.textContent = "File Browser";

    const closeBtn = document.createElement("button");
    closeBtn.className = "file-sidebar-close";
    closeBtn.setAttribute("aria-label", "Close file browser");
    closeBtn.innerHTML = CLOSE_SVG;

    header.appendChild(title);
    header.appendChild(closeBtn);

    const contentEl = document.createElement("div");
    contentEl.className = "file-sidebar-content";

    const handle = document.createElement("div");
    handle.className = "file-sidebar-handle";

    el.appendChild(header);
    el.appendChild(contentEl);
    el.appendChild(handle);

    function applyWidth() {
        if (collapsed) {
            el.classList.add("is-collapsed");
            el.style.width = `${COLLAPSED_WIDTH}px`;
            el.style.minWidth = `${COLLAPSED_WIDTH}px`;
        } else {
            el.classList.remove("is-collapsed");
            el.style.width = `${width}px`;
            el.style.minWidth = `${MIN_WIDTH}px`;
        }
        saveState({ width, collapsed });
    }

    closeBtn.addEventListener("click", () => {
        collapsed = true;
        applyWidth();
    });

    function expand() {
        collapsed = false;
        applyWidth();
    }

    function collapse() {
        collapsed = true;
        applyWidth();
    }

    function toggle() {
        collapsed = !collapsed;
        applyWidth();
    }

    // Resize via drag handle
    let dragging = false;
    let startX = 0;
    let startWidth = 0;

    const onPointerMove = (e) => {
        if (!dragging) return;
        const delta = startX - e.clientX;
        const maxWidth = Math.floor(window.innerWidth * 0.5);
        width = Math.max(MIN_WIDTH, Math.min(maxWidth, startWidth + delta));
        el.style.width = `${width}px`;
    };

    const onPointerUp = () => {
        if (!dragging) return;
        dragging = false;
        document.body.style.cursor = "";
        document.body.style.userSelect = "";
        el.classList.remove("is-resizing");
        saveState({ width, collapsed });
        window.removeEventListener("pointermove", onPointerMove);
        window.removeEventListener("pointerup", onPointerUp);
    };

    handle.addEventListener("pointerdown", (e) => {
        if (collapsed) return;
        e.preventDefault();
        dragging = true;
        startX = e.clientX;
        startWidth = width;
        document.body.style.cursor = "col-resize";
        document.body.style.userSelect = "none";
        el.classList.add("is-resizing");
        window.addEventListener("pointermove", onPointerMove);
        window.addEventListener("pointerup", onPointerUp);
    });

    applyWidth();

    return { el, contentEl, expand, collapse, toggle };
}
