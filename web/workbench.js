import { createDockview } from "./vendor/dockview/dockview-core.esm.js";

const STORAGE_KEY = "gitvista-workbench-dockview";
const PANEL_COMPONENT = "gitvista-panel";

const ICONS = {
    graph: `<svg width="20" height="20" viewBox="0 0 16 16" fill="currentColor">
        <path d="M2 2h2v2H2V2zm5 0h2v2H7V2zm5 0h2v2h-2V2zM2 7h2v2H2V7zm5 0h2v2H7V7zm5 0h2v2h-2V7zM2 12h2v2H2v-2zm5 0h2v2H7v-2zm5 0h2v2h-2v-2z"/>
    </svg>`,
    repository: `<svg width="20" height="20" viewBox="0 0 16 16" fill="currentColor">
        <path d="M2 2.5A2.5 2.5 0 014.5 0h8.75a.75.75 0 01.75.75v12.5a.75.75 0 01-.75.75h-2.5a.75.75 0 110-1.5h1.75v-2h-8a1 1 0 00-.714 1.7.75.75 0 01-1.072 1.05A2.495 2.495 0 012 11.5v-9zm10.5-1h-8a1 1 0 00-1 1v6.708A2.486 2.486 0 014.5 9h8V1.5zm-8 11a1 1 0 100-2 1 1 0 000 2z"/>
    </svg>`,
    "file-explorer": `<svg width="20" height="20" viewBox="0 0 16 16" fill="currentColor">
        <path d="M1.75 1A1.75 1.75 0 000 2.75v10.5C0 14.216.784 15 1.75 15h12.5A1.75 1.75 0 0016 13.25v-8.5A1.75 1.75 0 0014.25 3H7.5a.25.25 0 01-.2-.1l-.9-1.2C6.07 1.26 5.55 1 5 1H1.75zM1.5 2.75a.25.25 0 01.25-.25H5c.09 0 .176.04.232.107l.953 1.269c.381.508.97.806 1.596.806h6.469a.25.25 0 01.25.25v8.5a.25.25 0 01-.25.25H1.75a.25.25 0 01-.25-.25V2.75z"/>
    </svg>`,
    "three-zones": `<svg width="20" height="20" viewBox="0 0 16 16" fill="currentColor">
        <rect x="7.25" y="0" width="1.5" height="16" rx=".75"/>
        <circle cx="8" cy="2.5" r="2.5"/>
        <circle cx="8" cy="8" r="2.5"/>
        <circle cx="8" cy="13.5" r="2.5"/>
    </svg>`,
    analytics: `<svg width="20" height="20" viewBox="0 0 16 16" fill="currentColor">
        <rect x="1" y="8" width="3" height="7" rx=".75"/>
        <rect x="6.5" y="4" width="3" height="11" rx=".75"/>
        <rect x="12" y="1" width="3" height="14" rx=".75"/>
    </svg>`,
};

const panelRegistry = new Map();

class WorkbenchPanel {
    constructor(id, component) {
        this.id = id;
        this.component = component;
        this.element = document.createElement("div");
        this.element.className = "dockview-panel-host";
        this._params = null;
    }

    init(parameters) {
        this._params = parameters?.params ?? null;
        this.#render();
    }

    update(event) {
        if (event?.params) this._params = event.params;
        this.#render();
    }

    layout() {
        // no-op
    }

    focus() {
        this.element.focus?.();
    }

    toJSON() {
        return {};
    }

    dispose() {
        // no-op
    }

    #render() {
        const viewName = this._params?.viewName;
        if (!viewName) return;
        const view = panelRegistry.get(viewName);
        if (!view || !view.content) return;

        if (view.content.parentElement !== this.element) {
            this.element.innerHTML = "";
            this.element.appendChild(view.content);
        }

        if (typeof view.onShow === "function") {
            view.onShow();
        }
    }
}

function loadLayout() {
    try {
        const raw = localStorage.getItem(STORAGE_KEY);
        if (!raw) return null;
        return JSON.parse(raw);
    } catch {
        return null;
    }
}

function saveLayout(api) {
    try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(api.toJSON()));
    } catch {
        // ignore persistence failures
    }
}

function getThemeClass() {
    const explicit = document.documentElement.getAttribute("data-theme");
    if (explicit === "light") return "dockview-theme-light";
    if (explicit === "dark") return "dockview-theme-dark";

    // "system" mode: mirror OS preference so Dockview tabs/panels stay in sync.
    const prefersDark = window.matchMedia?.("(prefers-color-scheme: dark)")?.matches ?? false;
    return prefersDark ? "dockview-theme-dark" : "dockview-theme-light";
}

export function createWorkbench(views) {
    panelRegistry.clear();
    for (const view of views) panelRegistry.set(view.name, view);

    const viewMap = new Map(views.map((view) => [view.name, view]));

    const el = document.createElement("div");
    el.className = "workbench";

    const activityBar = document.createElement("div");
    activityBar.className = "activity-bar";

    const dockContainer = document.createElement("div");
    dockContainer.className = `workbench-dock ${getThemeClass()}`;

    el.appendChild(activityBar);
    el.appendChild(dockContainer);

    const api = createDockview(dockContainer, {
        disableFloatingGroups: true,
        noPanelsOverlay: "emptyGroup",
        singleTabMode: "default",
        createComponent: (options) => new WorkbenchPanel(options?.id, options?.component ?? PANEL_COMPONENT),
        components: {
            [PANEL_COMPONENT]: WorkbenchPanel,
        },
    });

    const iconButtons = new Map();
    const panelsByName = new Map();

    function listPanels() {
        if (!api?.panels) return [];
        if (Array.isArray(api.panels)) return api.panels;
        if (typeof api.panels.values === "function") return Array.from(api.panels.values());
        if (typeof api.panels.forEach === "function") {
            const items = [];
            api.panels.forEach((panel) => items.push(panel));
            return items;
        }
        return [];
    }

    function syncPanelsFromDockview() {
        panelsByName.clear();
        for (const panel of listPanels()) {
            if (panel?.id && viewMap.has(panel.id)) {
                panelsByName.set(panel.id, panel);
            }
        }
    }

    function updateActivitySelection() {
        const open = new Set(panelsByName.keys());
        for (const [name, btn] of iconButtons) {
            btn.classList.toggle("is-active", open.has(name));
        }
    }

    function openView(name) {
        const view = viewMap.get(name);
        if (!view) return;

        const existing = panelsByName.get(name);
        if (existing) {
            existing.api?.setActive?.();
            return;
        }

        const panel = api.addPanel({
            id: name,
            component: PANEL_COMPONENT,
            title: view.tooltip,
            params: { viewName: name },
        });

        panelsByName.set(name, panel);
        syncPanelsFromDockview();
        updateActivitySelection();
    }

    for (const view of views) {
        const btn = document.createElement("button");
        btn.className = "activity-bar-icon";
        btn.type = "button";
        btn.setAttribute("aria-label", view.tooltip);
        btn.innerHTML = ICONS[view.name] || view.icon || "";

        const tip = document.createElement("span");
        tip.className = "activity-bar-tooltip";
        tip.textContent = view.tooltip;
        btn.appendChild(tip);

        btn.addEventListener("click", () => openView(view.name));

        activityBar.appendChild(btn);
        iconButtons.set(view.name, btn);
    }

    api.onDidRemovePanel?.((event) => {
        const id = event?.panel?.id;
        if (id && panelsByName.has(id)) {
            syncPanelsFromDockview();
            updateActivitySelection();
        }
    });

    api.onDidActivePanelChange?.((event) => {
        const id = event?.panel?.id;
        if (!id) return;
        const view = viewMap.get(id);
        if (view && typeof view.onShow === "function") {
            view.onShow();
        }
    });

    api.onDidLayoutChange?.(() => {
        saveLayout(api);
        syncPanelsFromDockview();
        updateActivitySelection();
    });

    const initial = loadLayout();
    if (initial) {
        try {
            api.fromJSON(initial);
            syncPanelsFromDockview();
            updateActivitySelection();
        } catch {
            openView("graph");
        }
    } else {
        openView("graph");
    }

    function applyDockThemeClass() {
        dockContainer.classList.remove("dockview-theme-light", "dockview-theme-dark");
        dockContainer.classList.add(getThemeClass());
    }

    const observer = new MutationObserver(() => {
        applyDockThemeClass();
    });
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });

    const media = window.matchMedia?.("(prefers-color-scheme: dark)");
    const onSystemThemeChange = () => {
        // Only relevant when explicit theme is not set ("system" mode).
        if (!document.documentElement.hasAttribute("data-theme")) {
            applyDockThemeClass();
        }
    };
    media?.addEventListener?.("change", onSystemThemeChange);

    function destroy() {
        observer.disconnect();
        media?.removeEventListener?.("change", onSystemThemeChange);
        api?.dispose?.();
        panelsByName.clear();
        iconButtons.clear();
        panelRegistry.clear();
        el.remove();
    }

    return {
        el,
        isViewVisible: (name) => panelsByName.has(name),
        focusView: (name) => openView(name),
        getActiveViews: () => ({ primary: api.activePanel?.id ?? null, secondary: null }),
        destroy,
    };
}
