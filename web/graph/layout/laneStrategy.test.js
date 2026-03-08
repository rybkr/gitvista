import { describe, it } from "node:test";
import assert from "node:assert/strict";

import { LaneStrategy } from "./laneStrategy.js";

function makeCommit(hash, parents, when, message = hash, branchLabel = "", branchLabelSource = "") {
	return {
		hash,
		parents,
		message,
		author: { when },
		committer: { when },
		branchLabel,
		branchLabelSource,
	};
}

function makeNode(hash) {
	return {
		type: "commit",
		hash,
		x: 0,
		y: 0,
	};
}

describe("LaneStrategy lane header names", () => {
	it("prefers normalized branch ref names over tip hashes", () => {
		const strategy = new LaneStrategy();
		const commits = new Map([
			["a", makeCommit("a", [], "2026-03-01T00:00:00Z", "init")],
			["b", makeCommit("b", ["a"], "2026-03-02T00:00:00Z", "main work")],
			["f1", makeCommit("f1", ["b"], "2026-03-03T00:00:00Z", "feature work")],
			["m", makeCommit("m", ["b", "f1"], "2026-03-04T00:00:00Z", "Merge branch 'feature/security' into main")],
		]);
		const nodes = [...commits.keys()].map(makeNode);
		const branches = new Map([
			["refs/heads/main", "m"],
			["refs/remotes/origin/feature/security", "f1"],
		]);

		strategy.updateGraph(nodes, [], commits, branches, { height: 800 });

		const labels = strategy.getLaneInfo()
			.flatMap((lane) => lane.segments.map((seg) => seg.displayName))
			.filter(Boolean);
		const sources = strategy.getLaneInfo()
			.flatMap((lane) => lane.segments.map((seg) => seg.displayNameSource))
			.filter(Boolean);

		assert.ok(labels.includes("feature/security"));
		assert.ok(!labels.includes("refs/remotes/origin/feature/security"));
		assert.ok(!labels.includes("f1"));
		assert.ok(sources.includes("lane_owner") || sources.includes("ref"));
		assert.ok(!sources.includes("tip_hash"));
	});

	it("accepts branch metadata objects when deriving lane names", () => {
		const strategy = new LaneStrategy();
		const commits = new Map([
			["a", makeCommit("a", [], "2026-03-01T00:00:00Z", "init")],
			["b", makeCommit("b", ["a"], "2026-03-02T00:00:00Z", "main work")],
			["d1", makeCommit("d1", ["b"], "2026-03-03T00:00:00Z", "dev work")],
		]);
		const nodes = [...commits.keys()].map(makeNode);
		const branches = new Map([
			["refs/heads/dev", { target: "d1", type: "local" }],
			["refs/heads/main", { target: "b", type: "local" }],
		]);

		strategy.updateGraph(nodes, [], commits, branches, { height: 800 });

		const labels = strategy.getLaneInfo()
			.flatMap((lane) => lane.segments.map((seg) => seg.displayName))
			.filter(Boolean);
		const sources = strategy.getLaneInfo()
			.flatMap((lane) => lane.segments.map((seg) => seg.displayNameSource))
			.filter(Boolean);

		assert.ok(labels.includes("dev"));
		assert.ok(labels.includes("main"));
		assert.ok(!labels.includes("d1"));
		assert.ok(sources.includes("lane_owner") || sources.includes("ref"));
	});

	it("prefers backend branch labels over frontend fallback inference", () => {
		const strategy = new LaneStrategy();
		const commits = new Map([
			["a", makeCommit("a", [], "2026-03-01T00:00:00Z", "init", "main", "local_ref")],
			["b", makeCommit("b", ["a"], "2026-03-02T00:00:00Z", "main work", "main", "local_ref")],
			["d1", makeCommit("d1", ["b"], "2026-03-03T00:00:00Z", "dev work", "dev", "head_ref")],
		]);
		const nodes = [...commits.keys()].map(makeNode);
		const branches = new Map([
			["refs/heads/main", "b"],
			["refs/heads/dev", "d1"],
		]);

		strategy.updateGraph(nodes, [], commits, branches, { height: 800 });

		const labels = strategy.getLaneInfo()
			.flatMap((lane) => lane.segments.map((seg) => seg.displayName))
			.filter(Boolean);
		const sources = strategy.getLaneInfo()
			.flatMap((lane) => lane.segments.map((seg) => seg.displayNameSource))
			.filter(Boolean);

		assert.ok(labels.includes("dev"));
		assert.ok(sources.includes("head_ref"));
		assert.ok(!labels.includes("d1"));
	});

	it("infers a merged branch name from merge commit messages when no ref remains", () => {
		const strategy = new LaneStrategy();
		const commits = new Map([
			["a", makeCommit("a", [], "2026-03-01T00:00:00Z", "init")],
			["b", makeCommit("b", ["a"], "2026-03-02T00:00:00Z", "main work")],
			["f1", makeCommit("f1", ["b"], "2026-03-03T00:00:00Z", "feature work")],
			["m", makeCommit("m", ["b", "f1"], "2026-03-04T00:00:00Z", "Merge branch 'feature/security' into dev")],
		]);
		const nodes = [...commits.keys()].map(makeNode);
		const branches = new Map([
			["refs/heads/main", "m"],
		]);

		strategy.updateGraph(nodes, [], commits, branches, { height: 800 });

		const labels = strategy.getLaneInfo()
			.flatMap((lane) => lane.segments.map((seg) => seg.displayName))
			.filter(Boolean);
		const sources = strategy.getLaneInfo()
			.flatMap((lane) => lane.segments.map((seg) => seg.displayNameSource))
			.filter(Boolean);

		assert.ok(labels.includes("feature/security"));
		assert.ok(sources.includes("merge_message"));
	});

	it("uses a shortened hash as the final fallback label", () => {
		const strategy = new LaneStrategy();
		const hash = "1234567890abcdef1234567890abcdef12345678";
		const commits = new Map([
			[hash, makeCommit(hash, [], "2026-03-01T00:00:00Z", "orphaned work")],
		]);
		const nodes = [makeNode(hash)];
		const branches = new Map();

		strategy.updateGraph(nodes, [], commits, branches, { height: 800 });

		const labels = strategy.getLaneInfo()
			.flatMap((lane) => lane.segments.map((seg) => seg.displayName))
			.filter(Boolean);
		const sources = strategy.getLaneInfo()
			.flatMap((lane) => lane.segments.map((seg) => seg.displayNameSource))
			.filter(Boolean);

		assert.ok(labels.includes("1234567"));
		assert.ok(!labels.includes(hash));
		assert.ok(sources.includes("tip_hash"));
	});
});

describe("LaneStrategy lane isolation hit-testing", () => {
	it("returns a body hit for a single-commit side branch segment", () => {
		const strategy = new LaneStrategy();
		const commits = new Map([
			["a", makeCommit("a", [], "2026-03-01T00:00:00Z", "init")],
			["b", makeCommit("b", ["a"], "2026-03-02T00:00:00Z", "main work")],
			["f1", makeCommit("f1", ["b"], "2026-03-03T00:00:00Z", "feature work")],
			["m", makeCommit("m", ["b", "f1"], "2026-03-04T00:00:00Z", "Merge branch 'feature/security' into dev")],
		]);
		const nodes = [...commits.keys()].map(makeNode);
		const branches = new Map([
			["refs/heads/main", "m"],
		]);

		strategy.updateGraph(nodes, [], commits, branches, { height: 800 });

		const lane = strategy.getLaneInfo().find((entry) =>
			entry.segments.some((seg) => seg.displayName === "feature/security"),
		);
		assert.ok(lane);

		const seg = lane.segments.find((entry) => entry.displayName === "feature/security");
		assert.ok(seg);

		const x = strategy.positionToX(lane.position);
		const y = (seg.minY + seg.maxY) / 2;
		const hit = strategy.findLaneBodyAt(x, y);

		assert.ok(hit);
		assert.equal(hit.position, lane.position);
		assert.ok(hit.hashes.has("f1"));
	});
});
