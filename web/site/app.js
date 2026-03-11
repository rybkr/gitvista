import { logger } from "../logger.js";
import { initThemeToggle } from "../themeToggle.js";
import { setApiBase } from "../apiBase.js";
import { clearRoot, cleanupActiveView, bootstrapGraph } from "../gitvista/app.js";
import { parseHostedPath } from "../gitvista/routes.js";
import { createRepoLanding } from "./repoLanding.js";
import { createDocsView } from "./docsView.js";
import { createRepoLoadingView } from "./repoLoadingView.js";
import { PRODUCT_INFO } from "./hostedProduct.js";

let activeViewCleanup = null;
let navigationToken = 0;

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
            onRepoSelect: (id) => navigateToPath(`/repo/${id}/loading`),
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

    const showRepoLoading = ({ repoId, navigateToPath, replacePath, onRouteChange, initialStatus }) => {
        document.title = `${PRODUCT_INFO.name} Loading`;
        let destroyed = false;
        const view = createRepoLoadingView({ repoId, navigateToPath, replacePath, onRouteChange, initialStatus });
        root.appendChild(view.el);

        return () => {
            if (destroyed) return;
            destroyed = true;
            view.destroy();
            view.el.remove();
        };
    };

    const fetchRepoStatus = async (repoId) => {
        const resp = await fetch(`/api/repos/${repoId}/status`);
        if (!resp.ok) {
            let message = `Repository status request failed (${resp.status})`;
            try {
                const text = (await resp.text()).trim();
                if (text) message = text;
            } catch {
                // Ignore body parse failures.
            }
            throw new Error(message);
        }
        return resp.json();
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

    const navigateToHostedPath = (path) => {
        const nextPath = typeof path === "string" && path ? path : "/";
        if (location.pathname === nextPath && !location.search && !location.hash) return;
        history.pushState(null, "", nextPath);
        void mountFromLocation();
    };

    const mountFromLocation = async () => {
        const token = ++navigationToken;
        const parsed = parseHostedPath(location.pathname);
        if ((parsed.page === "repo" || parsed.page === "repo-loading") && parsed.repoId) {
            let status = null;
            try {
                status = await fetchRepoStatus(parsed.repoId);
            } catch (error) {
                status = {
                    id: parsed.repoId,
                    state: "error",
                    error: error.message || "Unable to load repository status.",
                    phase: "",
                    percent: 0,
                };
            }
            if (token !== navigationToken) return;

            if (status.state === "ready") {
                if (parsed.page === "repo-loading") {
                    replaceHostedPath(`/repo/${parsed.repoId}`);
                }
                document.title = PRODUCT_INFO.name;
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

            if (parsed.page === "repo") {
                replaceHostedPath(`/repo/${parsed.repoId}/loading`);
            }
            setApiBase("");
            mount(() => showRepoLoading({
                repoId: parsed.repoId,
                navigateToPath: navigateToHostedPath,
                replacePath: replaceHostedPath,
                onRouteChange: () => {
                    void mountFromLocation();
                },
                initialStatus: status,
            }));
            return;
        }
        if (token !== navigationToken) return;
        setApiBase("");
        if (parsed.page === "docs") {
            mount(() => showDocs(navigateToHostedPath, parsed.docsSection));
        } else {
            mount(() => showLanding(navigateToHostedPath));
        }
    };

    void mountFromLocation();
    window.addEventListener("popstate", () => {
        void mountFromLocation();
    });
});
