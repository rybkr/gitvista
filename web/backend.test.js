import { describe, it } from "node:test";
import assert from "node:assert/strict";

describe("startBackend", () => {
    it("routes typed websocket bootstrap messages to the right callbacks", async () => {
        globalThis.location = { protocol: "http:", host: "example.com" };

        const repoSummaries = [];
        const bootstrapChunks = [];
        const bootstrapComplete = [];
        const deltas = [];
        const statuses = [];
        const heads = [];

        let handlers = new Map();
        class FakeWebSocket {
            constructor(url) {
                this.url = url;
                handlers = new Map();
                queueMicrotask(() => handlers.get("open")?.({}));
            }

            addEventListener(type, handler) {
                handlers.set(type, handler);
            }

            close() {}
        }

        const originalWebSocket = globalThis.WebSocket;
        globalThis.WebSocket = FakeWebSocket;
        try {
            const { startBackend } = await import("./backend.js");
            const session = startBackend({
                onRepoMetadata: (payload) => repoSummaries.push(payload),
                onBootstrapChunk: (payload) => bootstrapChunks.push(payload),
                onBootstrapComplete: (payload) => bootstrapComplete.push(payload),
                onDelta: (payload) => deltas.push(payload),
                onStatus: (payload) => statuses.push(payload),
                onHead: (payload) => heads.push(payload),
            });

            await Promise.resolve();

            handlers.get("message")?.({
                data: JSON.stringify({ type: "repoSummary", repo: { name: "repo", commitCount: 5 } }),
            });
            handlers.get("message")?.({
                data: JSON.stringify({ type: "graphBootstrapChunk", bootstrap: { chunkIndex: 0, commits: [{ hash: "a" }] } }),
            });
            handlers.get("message")?.({
                data: JSON.stringify({ type: "bootstrapComplete", bootstrapComplete: { headHash: "a", tags: {} } }),
            });
            handlers.get("message")?.({
                data: JSON.stringify({ type: "graphDelta", delta: { addedCommits: [{ hash: "b" }] } }),
            });
            handlers.get("message")?.({
                data: JSON.stringify({ type: "status", status: { changed: [] } }),
            });
            handlers.get("message")?.({
                data: JSON.stringify({ type: "head", head: { hash: "a" } }),
            });

            assert.equal(repoSummaries.length, 1);
            assert.equal(repoSummaries[0].name, "repo");
            assert.equal(bootstrapChunks.length, 1);
            assert.equal(bootstrapChunks[0].chunkIndex, 0);
            assert.equal(bootstrapComplete.length, 1);
            assert.equal(bootstrapComplete[0].headHash, "a");
            assert.equal(deltas.length, 1);
            assert.equal(statuses.length, 1);
            assert.equal(heads.length, 1);

            session.destroy();
        } finally {
            globalThis.WebSocket = originalWebSocket;
        }
    });
});
