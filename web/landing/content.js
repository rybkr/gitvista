export const FEATURED_REPOS = [
    {
        url: "https://github.com/rybkr/gitvista",
        name: "rybkr/gitvista",
        description: "Git history visualization for branches, diffs, and activity",
    },
    {
        url: "https://github.com/jqlang/jq",
        name: "jqlang/jq",
        description: "Command-line JSON processor",
    },
];

export const HERO_PREVIEW = {
    path: "gitvista.io / repo / rybkr / gitvista",
    graph: {
        kicker: "Branch graph",
        title: "Follow the history shape before opening a commit.",
        summary: "Keep the active branch, side work, and merge point legible in one compact surface.",
        lanes: [
            {
                label: "main",
                tone: "main",
                commits: [
                    { hash: "f6a1c9e", title: "Release preview compact poster", meta: "HEAD", emphasis: "active" },
                    { hash: "9dc28b4", title: "Stabilize hosted repo bootstrap", meta: "2 commits back" },
                ],
            },
            {
                label: "preview-refresh",
                tone: "branch",
                commits: [
                    { hash: "4fe12d7", title: "Prototype graph-first onboarding", meta: "feature branch" },
                ],
            },
            {
                label: "merge",
                tone: "merge",
                commits: [
                    { hash: "82bd41f", title: "Merge preview-refresh into main", meta: "merge context" },
                ],
            },
        ],
    },
    focusCard: {
        kicker: "Focused commit",
        title: "Release preview compact poster",
        summary: "Once the lane context is obvious, drop into the exact commit that changed the landing experience.",
        pills: ["HEAD", "3 files changed"],
    },
    chips: ["branch graph", "recent activity", "commit diffs"],
};
