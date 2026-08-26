#!/usr/bin/env bun
/**
 * For each file in Notes/tasks, check the same-named file in Notes/issues
 * for the word "hitl" (case-insensitive). If found, skip the task file.
 * If not found, remove the block containing "**Type**: REVIEW" from the
 * task file. Blocks are the content between "---" separator lines; the
 * final "---" is preserved.
 */

import { readdir, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";

const ROOT = join(import.meta.dir, "..");
const TASKS_DIR = join(ROOT, "Notes", "tasks");
const ISSUES_DIR = join(ROOT, "Notes", "issues");

const REVIEW_RE = /\*\*type\*\*\s*:\s*review/i;
const HITL_RE = /hitl/i;
const SEPARATOR = /^---\s*$/;

async function main() {
  const entries = await readdir(TASKS_DIR);
  const taskFiles = entries.filter((f) => f.endsWith(".md"));

  for (const name of taskFiles) {
    const issuePath = join(ISSUES_DIR, name);
    let issueText: string;
    try {
      issueText = await readFile(issuePath, "utf8");
    } catch {
      console.warn(`skip: no matching issue file for ${name}`);
      continue;
    }

    if (HITL_RE.test(issueText)) {
      console.log(`skip: hitl found in issue ${name}`);
      continue;
    }

    const taskPath = join(TASKS_DIR, name);
    const text = await readFile(taskPath, "utf8");
    const lines = text.split("\n");

    // Find separator line indices (lines that are exactly "---").
    const sepIdx: number[] = [];
    for (let i = 0; i < lines.length; i++) {
      if (SEPARATOR.test(lines[i])) sepIdx.push(i);
    }

    if (sepIdx.length === 0) {
      console.log(`skip: no separators in ${name}`);
      continue;
    }

    // Blocks: content before first separator is block 0 (header + first task),
    // then each segment between separators is a block, and the content after
    // the last separator is a trailing block (usually empty).
    // Build block ranges as [startLine, endLine) in terms of line indices,
    // where each block excludes the separator lines themselves.
    const blocks: { start: number; end: number; content: string }[] = [];

    // Block 0: from start to first separator.
    blocks.push({
      start: 0,
      end: sepIdx[0],
      content: lines.slice(0, sepIdx[0]).join("\n"),
    });

    // Middle blocks: between separators.
    for (let i = 0; i < sepIdx.length - 1; i++) {
      const s = sepIdx[i] + 1;
      const e = sepIdx[i + 1];
      blocks.push({
        start: s,
        end: e,
        content: lines.slice(s, e).join("\n"),
      });
    }

    // Trailing block: after last separator.
    const lastSep = sepIdx[sepIdx.length - 1];
    blocks.push({
      start: lastSep + 1,
      end: lines.length,
      content: lines.slice(lastSep + 1).join("\n"),
    });

    // Find the REVIEW block (a middle block containing the REVIEW type).
    let removed = false;
    const kept: typeof blocks = [];
    for (let i = 0; i < blocks.length; i++) {
      const b = blocks[i];
      // Only consider non-header, non-trailing blocks (middle blocks).
      if (i > 0 && i < blocks.length - 1 && REVIEW_RE.test(b.content)) {
        removed = true;
        continue;
      }
      kept.push(b);
    }

    if (!removed) {
      console.log(`skip: no REVIEW block in ${name}`);
      continue;
    }

    // Reconstruct: join kept blocks with "\n---\n", ensuring the file ends
    // with the final "---" (the trailing block is expected to be empty).
    // The original file ended with "---\n", so the trailing empty block
    // guarantees the final separator is retained.
    const out = kept.map((b) => b.content).join("\n---\n");

    await writeFile(taskPath, out, "utf8");
    console.log(`edited: removed REVIEW block from ${name}`);
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
