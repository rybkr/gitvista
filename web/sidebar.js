const DEFAULT_WIDTH = 320;
const MIN_WIDTH = 180;
const COLLAPSED_WIDTH = 0;
const STORAGE_KEY = "gitvista-sidebar";

function loadState() {
    try {
        const raw = localStorage.getItem(STORAGE_KEY);
        if (raw) return JSON.parse(raw);
    } catch {
        // ignore
    }
    return { width: DEFAULT_WIDTH, collapsed: false };
}

function saveState(state) {
    try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
    } catch {
        // ignore
    }
}

const HAMBURGER_SVG = `<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
    <path d="M2 3h12M2 8h12M2 13h12" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
</svg>`;

export function createSidebar() {
    const saved = loadState();
    let width = saved.width ?? DEFAULT_WIDTH;
    let collapsed = saved.collapsed ?? false;

    // Main sidebar element
    const el = document.createElement("aside");
    el.className = "sidebar";

    const header = document.createElement("div");
    header.className = "sidebar-header";

    const toggleBtn = document.createElement("button");
    toggleBtn.className = "sidebar-toggle";
    toggleBtn.setAttribute("aria-label", "Collapse sidebar");
    toggleBtn.innerHTML = HAMBURGER_SVG;
    header.appendChild(toggleBtn);

    const content = document.createElement("div");
    content.className = "sidebar-content";

    const handle = document.createElement("div");
    handle.className = "sidebar-handle";

    el.appendChild(header);
    el.appendChild(content);
    el.appendChild(handle);

    // Floating expand button (lives outside sidebar, inside #root)
    const expandBtn = document.createElement("button");
    expandBtn.className = "sidebar-expand";
    expandBtn.setAttribute("aria-label", "Expand sidebar");
    expandBtn.innerHTML = HAMBURGER_SVG;

    function applyWidth() {
        if (collapsed) {
            el.classList.add("is-collapsed");
            el.style.width = `${COLLAPSED_WIDTH}px`;
            el.style.minWidth = `${COLLAPSED_WIDTH}px`;
            expandBtn.style.display = "inline-flex";
        } else {
            el.classList.remove("is-collapsed");
            el.style.width = `${width}px`;
            el.style.minWidth = `${MIN_WIDTH}px`;
            expandBtn.style.display = "none";
        }
        saveState({ width, collapsed });
    }

    toggleBtn.addEventListener("click", () => {
        collapsed = true;
        applyWidth();
    });

    expandBtn.addEventListener("click", () => {
        collapsed = false;
        applyWidth();
    });

    // Resize via drag handle
    let dragging = false;
    let startX = 0;
    let startWidth = 0;

    const onPointerMove = (e) => {
        if (!dragging) return;
        const delta = e.clientX - startX;
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

    return { el, expandBtn, content };
}
