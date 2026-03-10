import { logger } from "../logger.js";
import { initThemeToggle } from "../themeToggle.js";
import { setApiBase } from "../apiBase.js";
import { clearRoot, cleanupActiveView, bootstrapGraph } from "../gitvista/app.js";
import { parseHostedPath } from "../gitvista/routes.js";
import { createRepoLanding } from "./repoLanding.js";
import { createDocsView } from "./docsView.js";
import { createInstallView } from "./installView.js";
import { PRODUCT_INFO } from "./hostedProduct.js";

let activeViewCleanup = null;

document.addEventListener("DOMContentLoaded", () => {
    logger.info("Bootstrapping site frontend");
    initThemeToggle();

    const root = document.querySelector("#root");
    if (!root) {
        logger.error("Root element not found");
        return;
    }

    const mount = (factory) => {
        cleanupActiveView(activeViewCleanup);
        clearRoot(root);
        activeViewCleanup = factory();
    };

    const replaceHostedPath = (path) => {
        const nextPath = typeof path === "string" && path ? path : "/";
        history.replaceState(null, "", nextPath);
    };

    const showLanding = (navigateToPath) => {
        document.title = PRODUCT_INFO.name;
        let destroyed = false;
        const landing = createRepoLanding({
            onRepoSelect: (id) => navigateToPath(`/repo/${id}`),
            onNavigate: navigateToPath,
        });
        root.appendChild(landing.el);

        return () => {
            if (destroyed) return;
            destroyed = true;
            landing.destroy();
            landing.el.remove();
        };
    };

    const showDocs = (navigateToPath, activeSection) => {
        document.title = `${PRODUCT_INFO.name} Docs`;
        let destroyed = false;
        const docs = createDocsView({ navigateToPath, activeSection });
        root.appendChild(docs.el);

        return () => {
            if (destroyed) return;
            destroyed = true;
            docs.destroy();
            docs.el.remove();
        };
    };

    const showInstall = (navigateToPath) => {
        document.title = `${PRODUCT_INFO.name} Install`;
        let destroyed = false;
        const install = createInstallView({ navigateToPath });
        root.appendChild(install.el);

        return () => {
            if (destroyed) return;
            destroyed = true;
            install.destroy();
            install.el.remove();
        };
    };

    const navigateToHostedPath = (path) => {
        const nextPath = typeof path === "string" && path ? path : "/";
        if (location.pathname === nextPath && !location.search && !location.hash) return;
        history.pushState(null, "", nextPath);
        mountFromLocation();
    };

    const mountFromLocation = () => {
        const parsed = parseHostedPath(location.pathname);
        if (parsed.page === "repo" && parsed.repoId) {
            setApiBase(`/api/repos/${parsed.repoId}`);
            mount(() => bootstrapGraph(root, {
                repoId: parsed.repoId,
                navigateToPath: navigateToHostedPath,
                replacePath: replaceHostedPath,
                parsePermalinkHash: () => parseHostedPath(location.pathname)?.commitHash || null,
                productName: PRODUCT_INFO.name,
            }));
            return;
        }
        setApiBase("");
        if (parsed.page === "install") {
            mount(() => showInstall(navigateToHostedPath));
        } else if (parsed.page === "docs") {
            mount(() => showDocs(navigateToHostedPath, parsed.docsSection));
        } else {
            mount(() => showLanding(navigateToHostedPath));
        }
    };

    mountFromLocation();
    window.addEventListener("popstate", mountFromLocation);
});
