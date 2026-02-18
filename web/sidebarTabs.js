export function createSidebarTabs(tabs) {
    if (!tabs || tabs.length === 0) {
        throw new Error("createSidebarTabs requires at least one tab");
    }

    const container = document.createElement("div");
    container.style.display = "flex";
    container.style.flexDirection = "column";
    container.style.flex = "1";
    container.style.overflow = "hidden";

    const tabBar = document.createElement("div");
    tabBar.className = "sidebar-tabs";

    const contentContainer = document.createElement("div");
    contentContainer.style.display = "flex";
    contentContainer.style.flexDirection = "column";
    contentContainer.style.flex = "1";
    contentContainer.style.overflow = "hidden";

    let activeTabName = tabs[0].name;

    const tabButtons = new Map();
    const contentPanels = new Map();

    for (const tab of tabs) {
        const button = document.createElement("button");
        button.className = "sidebar-tab";
        button.textContent = tab.label;
        button.setAttribute("data-tab", tab.name);
        tabBar.appendChild(button);
        tabButtons.set(tab.name, button);

        const panel = document.createElement("div");
        panel.className = "sidebar-tab-content";
        panel.setAttribute("data-tab-content", tab.name);
        panel.appendChild(tab.content);
        contentContainer.appendChild(panel);
        contentPanels.set(tab.name, panel);

        button.addEventListener("click", () => {
            showTab(tab.name);
        });
    }

    function showTab(name) {
        if (!tabButtons.has(name)) {
            console.warn(`Tab "${name}" does not exist`);
            return;
        }

        activeTabName = name;

        for (const [tabName, button] of tabButtons.entries()) {
            button.classList.toggle("is-active", tabName === name);
        }

        for (const [tabName, panel] of contentPanels.entries()) {
            panel.classList.toggle("is-active", tabName === name);
        }
    }

    function getActiveTab() {
        return activeTabName;
    }

    showTab(activeTabName);

    container.appendChild(tabBar);
    container.appendChild(contentContainer);

    return {
        el: container,
        showTab,
        getActiveTab,
    };
}
