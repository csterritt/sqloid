#!/usr/bin/env bun
/**
 * json-watch.ts
 *
 * Tails a file and parses each new line as JSON. Prints extracted fields
 * depending on the message shape:
 *   - assistant/user/etc with content: id, timestamp, first 40 chars of content
 *   - toolResult: id, timestamp, toolName
 *   - anything else: id, timestamp, role
 *
 * Lines that fail JSON parsing print "Not JSON:" plus the first 40 chars.
 *
 * Usage: json-watch.ts <file>
 */

import { watchFile } from "node:fs";
import { open, stat } from "node:fs/promises";
import { resolve } from "node:path";

const file = process.argv[2];
if (!file) {
  console.error("Usage: json-watch.ts <file>");
  process.exit(1);
}

const path = resolve(file);

/** Extract the first 40 characters of a message's content array. */
function contentPreview(content: unknown): string {
  if (!Array.isArray(content) || content.length === 0) return "";
  const first = content[0] as Record<string, unknown> | undefined;
  if (!first) return "";
  // Content items may carry text in "text" or "thinking" fields.
  const text = (first.text ?? first.thinking ?? "") as string;
  return text.replace(/\n/g, " ").slice(0, 40);
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

async function main() {
  let size = 0;
  let buf = "";
  let fd: Awaited<ReturnType<typeof open>> | undefined;

  // On an existing file, show the last 5 lines before tailing.
  try {
    const s = await stat(path);
    const fd0 = await open(path, "r");
    const data = Buffer.alloc(s.size);
    await fd0.read(data, 0, s.size, 0);
    await fd0.close();
    const allLines = data.toString("utf8").split("\n").filter((l) => l.trim().length > 0);
    for (const line of allLines.slice(-5)) {
      handleLine(line);
    }
    size = s.size;
  } catch {
    // File may not exist yet; start at 0.
  }

  async function readNew() {
    try {
      const s = await stat(path);
      if (s.size < size) {
        // File was truncated/rotated; reset.
        size = 0;
        buf = "";
      }
      if (s.size === size) return;
      if (!fd) fd = await open(path, "r");
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

  await readNew();
  watchFile(path, { interval: 200 }, () => {
    readNew().catch(() => {});
  });

  console.log(`Watching ${path}...`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
