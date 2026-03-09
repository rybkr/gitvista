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
    problem: {
        kicker: "Without GitVista",
        title: "Git feels like fragments.",
        body: "Detached HEADs, branch names, and merge questions arrive as separate clues instead of one readable story.",
        lines: [
            "refs/remotes/origin/release/2.4 -> 0a71f8c",
            "merge preview-refresh into main?",
            "HEAD detached at 50c9298",
        ],
    },
    solution: {
        kicker: "With GitVista",
        title: "The repository reads like a map.",
        graph: {
            label: "Branch graph",
            summary: "See where work diverged, what rejoined, and which commit deserves inspection next.",
            lanes: [
                {
                    label: "main",
                    tone: "main",
                    commits: [
                        { hash: "f6a1c9e", title: "Release preview panel refresh", meta: "HEAD", emphasis: "active" },
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
        inspector: {
            label: "Focused commit",
            title: "Release preview panel refresh",
            summary: "Inspect one commit only after the branch context is obvious.",
            pills: ["HEAD", "3 files changed", "+128 -32"],
        },
        diff: {
            label: "Diff context",
            file: "web/repoLanding.js",
            stats: ["+82", "-18", "preview helpers"],
            excerpt: [
                "createHeroPreview(HERO_PREVIEW)",
                "createPreviewGraphLane(lane)",
                "highlightElementTemporarily(card)",
            ],
        },
        checklist: [
            { title: "Graph-first orientation", body: "Branch motion lands before commit detail." },
            { title: "One rail for context", body: "HEAD, lane, and diff stay visible together." },
            { title: "Commit detail on demand", body: "Open the diff when the why is already clear." },
        ],
        chips: ["branch graph", "activity context", "commit diffs"],
    },
};

