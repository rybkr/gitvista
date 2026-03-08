/**
 * Returns a human-friendly branch label for local and remote refs.
 *
 * @param {string} name
 * @returns {string}
 */
export function friendlyBranchName(name) {
    if (!name) return "";
    if (name.startsWith("refs/heads/")) return name.slice("refs/heads/".length);
    if (name.startsWith("refs/remotes/")) return name.slice("refs/remotes/".length);
    if (name.startsWith("refs/")) return name.slice("refs/".length);
    return name;
}
