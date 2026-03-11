import { logger } from "../logger.js";
import { initThemeToggle } from "../themeToggle.js";
import { setApiBase } from "../apiBase.js";
import { clearRoot, cleanupActiveView, bootstrapGraph } from "../gitvista/app.js";
import { buildHostedRepoApiBase, buildHostedRepoLoadingPath, buildHostedRepoPath, parseHostedPath } from "../gitvista/routes.js";
import { createRepoLanding } from "./repoLanding.js";
import { createDocsView } from "./docsView.js";
import { createRepoLoadingView } from "./repoLoadingView.js";
import { PRODUCT_INFO } from "./hostedProduct.js";
import { getHostedRepoAccess } from "./hostedAccess.js";

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
            accountSlug: parseHostedPath(location.pathname).accountSlug,
            onRepoSelect: (id, accountSlug) => navigateToPath(buildHostedRepoLoadingPath(accountSlug, id)),
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

    const showRepoLoading = ({ accountSlug, repoId, navigateToPath, replacePath, onRouteChange, initialStatus }) => {
        document.title = `${PRODUCT_INFO.name} Loading`;
        let destroyed = false;
        const view = createRepoLoadingView({ accountSlug, repoId, navigateToPath, replacePath, onRouteChange, initialStatus });
        root.appendChild(view.el);

        return () => {
            if (destroyed) return;
            destroyed = true;
            view.destroy();
            view.el.remove();
        };
    };

    const fetchRepoStatus = async (accountSlug, repoId) => {
        const access = getHostedRepoAccess(repoId);
        if (!access?.accessToken) {
            throw new Error("This browser no longer has access to that repository.");
        }
        const resp = await fetch(`${buildHostedRepoApiBase(accountSlug, repoId)}/status`, {
            headers: { "X-GitVista-Repo-Token": access.accessToken },
        });
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
                status = await fetchRepoStatus(parsed.accountSlug, parsed.repoId);
            } catch (error) {
                status = {
                    id: parsed.repoId,
                    accountId: parsed.accountSlug,
                    state: "error",
                    error: error.message || "Unable to load repository status.",
                    phase: "",
                    percent: 0,
                };
            }
            if (token !== navigationToken) return;

            if (status.state === "ready") {
                if (parsed.page === "repo-loading") {
                    replaceHostedPath(buildHostedRepoPath(parsed.accountSlug, parsed.repoId));
                }
                document.title = PRODUCT_INFO.name;
                setApiBase(
                    buildHostedRepoApiBase(parsed.accountSlug, parsed.repoId),
                    getHostedRepoAccess(parsed.repoId)?.accessToken || "",
                );
                mount(() => bootstrapGraph(root, {
                    repoId: parsed.repoId,
                    repoPathBuilder: (repoId, hash) => buildHostedRepoPath(parsed.accountSlug, repoId, hash),
                    navigateToPath: navigateToHostedPath,
                    replacePath: replaceHostedPath,
                    parsePermalinkHash: () => parseHostedPath(location.pathname)?.commitHash || null,
                    productName: PRODUCT_INFO.name,
                }));
                return;
            }

            if (parsed.page === "repo") {
                replaceHostedPath(buildHostedRepoLoadingPath(parsed.accountSlug, parsed.repoId));
            }
            setApiBase("", "");
            mount(() => showRepoLoading({
                accountSlug: parsed.accountSlug,
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
        setApiBase("", "");
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
