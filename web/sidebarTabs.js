/**
 * Sidebar tab system for managing multiple content panels.
 *
 * Creates a tab bar with buttons that switch between different content views.
 * Only one tab is active at a time, controlling which content panel is visible.
 */

export function createSidebarTabs(tabs) {
    if (!tabs || tabs.length === 0) {
        throw new Error("createSidebarTabs requires at least one tab");
    }

    const container = document.createElement("div");
    container.style.display = "flex";
    container.style.flexDirection = "column";
    container.style.flex = "1";
    container.style.overflow = "hidden";

    // Tab bar with buttons
    const tabBar = document.createElement("div");
    tabBar.className = "sidebar-tabs";

    // Content panels container
    const contentContainer = document.createElement("div");
    contentContainer.style.display = "flex";
    contentContainer.style.flexDirection = "column";
    contentContainer.style.flex = "1";
    contentContainer.style.overflow = "hidden";

    let activeTabName = tabs[0].name;

    // Create tab buttons and content panels
    const tabButtons = new Map();
    const contentPanels = new Map();

    for (const tab of tabs) {
        // Create tab button
        const button = document.createElement("button");
        button.className = "sidebar-tab";
        button.textContent = tab.label;
        button.setAttribute("data-tab", tab.name);
        tabBar.appendChild(button);
        tabButtons.set(tab.name, button);

        // Create content wrapper
        const panel = document.createElement("div");
        panel.className = "sidebar-tab-content";
        panel.setAttribute("data-tab-content", tab.name);
        panel.appendChild(tab.content);
        contentContainer.appendChild(panel);
        contentPanels.set(tab.name, panel);

        // Tab click handler
        button.addEventListener("click", () => {
            showTab(tab.name);
        });
    }

    function showTab(name) {
        // Validate tab exists
        if (!tabButtons.has(name)) {
            console.warn(`Tab "${name}" does not exist`);
            return;
        }

        activeTabName = name;

        // Update button states
        for (const [tabName, button] of tabButtons.entries()) {
            button.classList.toggle("is-active", tabName === name);
        }

        // Update content visibility
        for (const [tabName, panel] of contentPanels.entries()) {
            panel.classList.toggle("is-active", tabName === name);
        }
    }

    function getActiveTab() {
        return activeTabName;
    }

    // Initialize first tab as active
    showTab(activeTabName);

    container.appendChild(tabBar);
    container.appendChild(contentContainer);

    return {
        el: container,
        showTab,
        getActiveTab,
    };
}
