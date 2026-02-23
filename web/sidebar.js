const DEFAULT_WIDTH = 280;
const MIN_WIDTH = 200;
const STORAGE_KEY = "gitvista-sidebar";

function loadState() {
    try {
        const raw = localStorage.getItem(STORAGE_KEY);
        if (!raw) return { activePanel: null, panelWidth: DEFAULT_WIDTH };
        const parsed = JSON.parse(raw);
        // Migrate from old { width, collapsed } format
        if ("collapsed" in parsed && !("activePanel" in parsed)) {
            return {
                activePanel: parsed.collapsed ? null : "repository",
                panelWidth: parsed.width ?? DEFAULT_WIDTH,
            };
        }
        return {
            activePanel: parsed.activePanel ?? null,
            panelWidth: parsed.panelWidth ?? DEFAULT_WIDTH,
        };
    } catch {
        return { activePanel: null, panelWidth: DEFAULT_WIDTH };
    }
}

function saveState(state) {
    try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
    } catch {
        // ignore
    }
}

const ICONS = {
    repository: `<svg width="20" height="20" viewBox="0 0 16 16" fill="currentColor">
        <path d="M2 2.5A2.5 2.5 0 014.5 0h8.75a.75.75 0 01.75.75v12.5a.75.75 0 01-.75.75h-2.5a.75.75 0 110-1.5h1.75v-2h-8a1 1 0 00-.714 1.7.75.75 0 01-1.072 1.05A2.495 2.495 0 012 11.5v-9zm10.5-1h-8a1 1 0 00-1 1v6.708A2.486 2.486 0 014.5 9h8V1.5zm-8 11a1 1 0 100-2 1 1 0 000 2z"/>
    </svg>`,
    "file-explorer": `<svg width="20" height="20" viewBox="0 0 16 16" fill="currentColor">
        <path d="M1.75 1A1.75 1.75 0 000 2.75v10.5C0 14.216.784 15 1.75 15h12.5A1.75 1.75 0 0016 13.25v-8.5A1.75 1.75 0 0014.25 3H7.5a.25.25 0 01-.2-.1l-.9-1.2C6.07 1.26 5.55 1 5 1H1.75zM1.5 2.75a.25.25 0 01.25-.25H5c.09 0 .176.04.232.107l.953 1.269c.381.508.97.806 1.596.806h6.469a.25.25 0 01.25.25v8.5a.25.25 0 01-.25.25H1.75a.25.25 0 01-.25-.25V2.75z"/>
    </svg>`,
    "three-zones": `<svg width="20" height="20" viewBox="0 0 16 16" fill="currentColor">
        <path d="M1 2.5A1.5 1.5 0 012.5 1h2A1.5 1.5 0 016 2.5v11A1.5 1.5 0 014.5 15h-2A1.5 1.5 0 011 13.5v-11zm5.5 0A1.5 1.5 0 018 1h2a1.5 1.5 0 011.5 1.5v11A1.5 1.5 0 0110 15H8a1.5 1.5 0 01-1.5-1.5v-11zm5.5 0A1.5 1.5 0 0113.5 1h.5a1 1 0 011 1v12a1 1 0 01-1 1h-.5a1.5 1.5 0 01-1.5-1.5v-11z"/>
    </svg>`,
};

const CHEVRON_LEFT = `<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
    <path d="M10 12L6 8l4-4"/>
</svg>`;

export function createSidebar(panels) {
    const saved = loadState();
    let activePanel = saved.activePanel;
    let panelWidth = saved.panelWidth;

    // ── Activity Bar ──
    const activityBar = document.createElement("div");
    activityBar.className = "activity-bar";

    const iconButtons = new Map();

    for (const p of panels) {
        const btn = document.createElement("button");
        btn.className = "activity-bar-icon";
        btn.setAttribute("aria-label", p.tooltip);
        btn.setAttribute("data-panel", p.name);
        btn.innerHTML = ICONS[p.name] || p.icon || "";

        const tip = document.createElement("span");
        tip.className = "activity-bar-tooltip";
        tip.textContent = p.tooltip;
        btn.appendChild(tip);

        btn.addEventListener("click", () => {
            if (activePanel === p.name) {
                closePanel();
            } else {
                showPanel(p.name);
            }
        });

        activityBar.appendChild(btn);
        iconButtons.set(p.name, btn);
    }

    // ── Sidebar Panel ──
    const panel = document.createElement("div");
    panel.className = "sidebar-panel";

    const panelHeader = document.createElement("div");
    panelHeader.className = "sidebar-panel-header";

    const panelTitle = document.createElement("span");
    panelTitle.className = "sidebar-panel-title";

    const closeBtn = document.createElement("button");
    closeBtn.className = "sidebar-panel-close";
    closeBtn.setAttribute("aria-label", "Close panel");
    closeBtn.innerHTML = CHEVRON_LEFT;
    closeBtn.addEventListener("click", () => closePanel());

    panelHeader.appendChild(panelTitle);
    panelHeader.appendChild(closeBtn);

    const panelContent = document.createElement("div");
    panelContent.className = "sidebar-panel-content";

    // Add all content elements to the panel (hidden by default via CSS)
    const contentElements = new Map();
    for (const p of panels) {
        const wrapper = document.createElement("div");
        wrapper.className = "sidebar-panel-tab-content";
        wrapper.setAttribute("data-panel", p.name);
        wrapper.appendChild(p.content);
        panelContent.appendChild(wrapper);
        contentElements.set(p.name, wrapper);
    }

    const handle = document.createElement("div");
    handle.className = "sidebar-panel-handle";

    panel.appendChild(panelHeader);
    panel.appendChild(panelContent);
    panel.appendChild(handle);

    // ── State management ──

    function applyState() {
        // Update icon highlights
        for (const [name, btn] of iconButtons) {
            btn.classList.toggle("is-active", name === activePanel);
        }
        // Update content visibility
        for (const [name, el] of contentElements) {
            el.classList.toggle("is-active", name === activePanel);
        }

        if (activePanel) {
            panel.classList.add("is-open");
            panel.style.width = `${panelWidth}px`;
            const p = panels.find((p) => p.name === activePanel);
            panelTitle.textContent = p ? p.tooltip : "";
        } else {
            panel.classList.remove("is-open");
            panel.style.width = "0px";
        }

        saveState({ activePanel, panelWidth });
    }

    function showPanel(name) {
        if (!iconButtons.has(name)) return;
        activePanel = name;
        applyState();
    }

    function closePanel() {
        activePanel = null;
        applyState();
    }

    function getActivePanel() {
        return activePanel;
    }

    // ── Resize handling ──
    let dragging = false;
    let startX = 0;
    let startWidth = 0;

    const onPointerMove = (e) => {
        if (!dragging) return;
        const delta = e.clientX - startX;
        const activityBarWidth = 48;
        const maxWidth = Math.floor((window.innerWidth - activityBarWidth) * 0.5);
        panelWidth = Math.max(MIN_WIDTH, Math.min(maxWidth, startWidth + delta));
        panel.style.width = `${panelWidth}px`;
    };

    const onPointerUp = () => {
        if (!dragging) return;
        dragging = false;
        document.body.style.cursor = "";
        document.body.style.userSelect = "";
        panel.classList.remove("is-resizing");
        saveState({ activePanel, panelWidth });
        window.removeEventListener("pointermove", onPointerMove);
        window.removeEventListener("pointerup", onPointerUp);
    };

    handle.addEventListener("pointerdown", (e) => {
        if (!activePanel) return;
        e.preventDefault();
        dragging = true;
        startX = e.clientX;
        startWidth = panelWidth;
        document.body.style.cursor = "col-resize";
        document.body.style.userSelect = "none";
        panel.classList.add("is-resizing");
        window.addEventListener("pointermove", onPointerMove);
        window.addEventListener("pointerup", onPointerUp);
    });

    handle.addEventListener("dblclick", () => {
        if (!activePanel) return;
        panelWidth = DEFAULT_WIDTH;
        panel.style.width = `${panelWidth}px`;
        saveState({ activePanel, panelWidth });
    });

    // ── Initialize ──
    applyState();

    return { activityBar, panel, showPanel, closePanel, getActivePanel };
}
