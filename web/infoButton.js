export function createInfoButton({ text, id, classPrefix }) {
    const wrap = document.createElement("span");
    wrap.className = `${classPrefix}-help`;

    const button = document.createElement("button");
    button.type = "button";
    button.className = `${classPrefix}-help-button`;
    button.setAttribute("aria-label", `Explain ${id}`);
    button.setAttribute("aria-describedby", `${classPrefix}-help-${id}`);
    button.textContent = "i";

    const tooltip = document.createElement("span");
    tooltip.className = `${classPrefix}-help-tooltip`;
    tooltip.id = `${classPrefix}-help-${id}`;
    tooltip.setAttribute("role", "tooltip");
    tooltip.textContent = text;

    wrap.appendChild(button);
    wrap.appendChild(tooltip);
    return wrap;
}
