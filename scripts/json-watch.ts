#!/usr/bin/env bun
/**
 * json-watch.ts
 *
 * Watches a directory for the most recently modified file, tails it, and
 * parses each new line as JSON. Prints extracted fields depending on the
 * message shape:
 *   - assistant/user/etc with content: id, timestamp, first 40 chars of content
 *   - toolResult: id, timestamp, toolName
 *   - anything else: id, timestamp, role
 *
 * Lines that fail JSON parsing print "Not JSON:" plus the first 40 chars.
 *
 * Periodically checks the directory for a newer file. When a newer file is
 * found, prints a line of 20 hyphens and switches to tailing that file
 * instead.
 *
 * Usage: json-watch.ts <directory>
 */

import { unwatchFile, watchFile } from "node:fs";
import { open, stat, readdir } from "node:fs/promises";
import { resolve, join } from "node:path";

const dirArg = process.argv[2];
if (!dirArg) {
  console.error("Usage: json-watch.ts <directory>");
  process.exit(1);
}

const dir = resolve(dirArg);

/** How often to scan the directory for a newer file (ms). */
const DIR_SCAN_INTERVAL = 5000;

/** Extract the first 40 characters of a message's content array. */
function contentPreview(content: unknown): string {
  if (!Array.isArray(content) || content.length === 0) return "";
  const first = content[0] as Record<string, unknown> | undefined;
  if (!first) return "";
  // Content items may carry text in "text" or "thinking" fields.
  const text = (first.text ?? first.thinking ?? "") as string;
  return text.replace(/[\n]+/g, " ").slice(0, 40);
}

function handleLine(line: string): void {
  const trimmed = line.trim();
  if (trimmed.length === 0) return;

  let obj: Record<string, unknown>;
  try {
    obj = JSON.parse(trimmed);
  } catch {
    console.log(`Not JSON: ${trimmed.slice(0, 40)}`);
    return;
  }

  const id = obj.id ?? "?";
  const ts = obj.timestamp ?? "?";
  const msg = obj.message as Record<string, unknown> | undefined;

  if (msg) {
    const role = msg.role as string | undefined;

    if (role === "toolResult") {
      const toolName = msg.toolName ?? "?";
      console.log(`${ts} ${id} tool=${toolName}`);
      return;
    }

    if (role === "assistant" || role === "user") {
      const preview = contentPreview(msg.content);
      console.log(`${ts} ${id} ${preview}`);
      return;
    }

    console.log(`${ts} ${id} role=${role ?? "?"}`);
    return;
  }

  console.log(`${ts} ${id} (no message)`);
}

/** Find the most recently modified file in a directory. */
async function newestFile(): Promise<string | undefined> {
  let entries: string[];
  try {
    entries = await readdir(dir);
  } catch {
    return undefined;
  }
  let best: { path: string; mtime: number } | undefined;
  for (const name of entries) {
    const full = join(dir, name);
    try {
      const s = await stat(full);
      if (!s.isFile()) continue;
      if (!best || s.mtimeMs > best.mtime) {
        best = { path: full, mtime: s.mtimeMs };
      }
    } catch {
      // skip unreadable entries
    }
  }
  return best?.path;
}

async function main() {
  let currentPath = await newestFile();
  let size = 0;
  let buf = "";
  let fd: Awaited<ReturnType<typeof open>> | undefined;

  /** Read and print the last 5 lines of a file; sets `size` to its length. */
  async function loadInitial(path: string) {
    try {
      const s = await stat(path);
      const fd0 = await open(path, "r");
      const data = Buffer.alloc(s.size);
      await fd0.read(data, 0, s.size, 0);
      await fd0.close();
      const allLines = data
        .toString("utf8")
        .split("\n")
        .filter((l) => l.trim().length > 0);
      for (const line of allLines.slice(-5)) {
        handleLine(line);
      }
      size = s.size;
    } catch {
      // File may not exist yet; start at 0.
    }
  }

  /** Read any bytes appended to currentPath since the last read. */
  async function readNew() {
    if (!currentPath) return;
    try {
      const s = await stat(currentPath);
      if (s.size < size) {
        // File was truncated/rotated; reset.
        size = 0;
        buf = "";
      }
      if (s.size === size) return;
      if (!fd) fd = await open(currentPath, "r");
      const length = s.size - size;
      const data = Buffer.alloc(length);
      await fd.read(data, 0, length, size);
      size = s.size;
      buf += data.toString("utf8");
      let nl: number;
      while ((nl = buf.indexOf("\n")) >= 0) {
        const line = buf.slice(0, nl);
        buf = buf.slice(nl + 1);
        handleLine(line);
      }
    } catch {
      // Ignore transient errors (file deleted, etc.).
    }
  }

  /** Stop watching the current file and begin tailing `path` instead. */
  async function switchTo(path: string) {
    // Remove the previous watcher. unwatchFile clears all listeners for the
    // path (both Bun and Node support this).
    if (currentPath) {
      try {
        unwatchFile(currentPath);
      } catch {
        // ignore
      }
    }
    if (fd) {
      await fd.close().catch(() => {});
      fd = undefined;
    }
    size = 0;
    buf = "";
    currentPath = path;
    console.log("--------------------");
    await loadInitial(path);
    await readNew();
    watchFile(path, { interval: 200 }, () => {
      readNew().catch(() => {});
    });
  }

  if (currentPath) {
    await loadInitial(currentPath);
    await readNew();
    watchFile(currentPath, { interval: 200 }, () => {
      readNew().catch(() => {});
    });
    console.log(`Watching ${currentPath}...`);
  } else {
    console.log(`No files found in ${dir}; waiting...`);
  }

  // Periodically scan for a newer file.
  setInterval(async () => {
    const latest = await newestFile();
    if (!latest) return;
    if (latest !== currentPath) {
      await switchTo(latest).catch(() => {});
      console.log(`Watching ${latest}...`);
    }
  }, DIR_SCAN_INTERVAL);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
