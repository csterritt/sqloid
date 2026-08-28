#!/usr/bin/env bun
/**
 * build-tasks-until-review.ts
 *
 * Drives the sqloid task pipeline with a pi.dev agent session, looping
 * over unimplemented tasks until a REVIEW task is reached or an error
 * occurs:
 *
 *   1. Scan Notes/walkthroughs for subdirectories whose first three
 *      characters name an already-implemented task.
 *   2. Find the next Notes/tasks file (sorted) whose three-character
 *      numeric prefix has no walkthrough yet.
 *   3. Invoke a pi agent session (runPrintMode) to implement that task,
 *      pointing the agent at Notes/skills/code-writing for coding,
 *      comment, and documentation standards.
 *   4. Verify that all the go tests pass
 *   5. Verify the agent wrote a walkthrough: a new Notes/walkthroughs
 *      subdirectory starting with the task prefix containing a new file
 *      at least ten lines long. If not, notify via /home/chris/notify-app.
 *   6. On success, describe the current jj commit and start a fresh one:
 *        jj describe -m "Task <task-name> implemented by <PI_MODEL>."
 *        jj new
 *   7. If the task file contains a "**Type**: REVIEW" section, notify and
 *      stop the loop for human review. Otherwise, repeat from step 1.
 *
 * Usage:
 *   build-tasks-until-review.ts [--dry-run]
 *
 * --dry-run prints the planned actions without invoking pi, jj, or
 * notify-app, and without modifying any state.
 */

import { type Dirent } from "node:fs";
import { readdir, readFile, stat } from "node:fs/promises";
import { join } from "node:path";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import {
  type CreateAgentSessionRuntimeFactory,
  createAgentSessionFromServices,
  createAgentSessionRuntime,
  createAgentSessionServices,
  getAgentDir,
  resolveCliModel,
  SessionManager,
} from "@earendil-works/pi-coding-agent";

const execFileP = promisify(execFile);

process.on("SIGUSR1", () => {
  console.log("--- Process Interrupted: Current Stack Trace ---");
  console.trace();
});

const ROOT = join(import.meta.dir, "..");
const TASKS_DIR = join(ROOT, "Notes", "tasks");
const WALKTHROUGHS_DIR = join(ROOT, "Notes", "walkthroughs");
const SKILLS_CODE_WRITING_DIR = join(ROOT, "Notes", "skills", "code-writing");
const NOTIFY_APP = "/home/chris/notify-app";
const STOP_FILE = join(ROOT, "stop");

/** Model the pi agent uses, as a "<provider>/<model>" or "<provider>/<model>:<thinking>" string. */
const PI_MODEL = "z-ai/glm-5.3-flash";

/** Model name without any ":<thinking>" suffix, for use in commit messages. */
const PI_MODEL_NAME = PI_MODEL.split(":")[0];

const DRY_RUN = process.argv.includes("--dry-run");

const PREFIX_RE = /^(\d{3})/;

/** Matches a "**Type**: REVIEW" section in a task file (case-insensitive). */
const REVIEW_RE = /\*\*type\*\*\s*:\s*review/i;

/** Run a command in ROOT, returning trimmed stdout. Throws on non-zero exit. */
async function run(cmd: string, args: string[]): Promise<string> {
  const { stdout } = await execFileP(cmd, args, { cwd: ROOT });
  return stdout.toString().trim();
}

/** Log a line with a "dd HH:MM" timestamp prefix. */
function log(msg: string): void {
  const now = new Date();
  const ts = `${String(now.getMonth() + 1)}/${String(now.getDate()).padStart(2, "0")} ${String(now.getHours()).padStart(2, "0")}:${String(now.getMinutes()).padStart(2, "0")}`;
  console.log(`[${ts}] ${msg}`);
}

/** Recursively list all files under a directory as relative POSIX paths. */
async function listFilesDeep(dir: string): Promise<Set<string>> {
  const out = new Set<string>();
  async function walk(rel: string): Promise<void> {
    let entries: string[];
    try {
      entries = await readdir(join(dir, rel));
    } catch {
      return;
    }
    for (const name of entries) {
      const relChild = rel ? `${rel}/${name}` : name;
      const s = await stat(join(dir, relChild));
      if (s.isDirectory()) {
        await walk(relChild);
      } else {
        out.add(relChild);
      }
    }
  }
  await walk("");
  return out;
}

/** Top-level subdirectory names of a directory. */
async function topSubdirs(dir: string): Promise<string[]> {
  let entries: Dirent[];
  try {
    entries = await readdir(dir, { withFileTypes: true });
  } catch {
    return [];
  }
  return entries.filter((e) => e.isDirectory()).map((e) => e.name);
}

/** Numeric three-character prefixes of implemented tasks, from walkthroughs. */
async function implementedPrefixes(): Promise<Set<string>> {
  const dirs = await topSubdirs(WALKTHROUGHS_DIR);
  const prefixes = new Set<string>();
  for (const name of dirs) {
    const m = name.match(PREFIX_RE);
    if (m) prefixes.add(m[1]);
  }
  return prefixes;
}

/** Sorted task files in Notes/tasks with their numeric prefix and base name. */
async function taskFiles(): Promise<{ file: string; prefix: string; name: string }[]> {
  const entries = await readdir(TASKS_DIR);
  const out: { file: string; prefix: string; name: string }[] = [];
  for (const file of entries) {
    if (!file.endsWith(".md")) continue;
    const m = file.match(PREFIX_RE);
    if (!m) continue;
    out.push({ file, prefix: m[1], name: file.replace(/\.md$/, "") });
  }
  out.sort((a, b) => a.file.localeCompare(b.file));
  return out;
}

/** Find the next unimplemented task. */
async function nextTask() {
  const done = await implementedPrefixes();
  const tasks = await taskFiles();
  for (const t of tasks) {
    if (!done.has(t.prefix)) return t;
  }
  return undefined;
}

/** Build the prompt sent to the pi agent for a task. */
function buildPrompt(task: { file: string; prefix: string; name: string }, taskText: string): string {
  return [
    `Implement the sqloid task described in Notes/tasks/${task.file}.`,
    ``,
    `The full task file content is below:`,
    ``,
    taskText,
    ``,
    `Before writing any code, read and follow the coding, comment, and`,
    `documentation standards in Notes/skills/code-writing/ (see each file in`,
    `that directory). Reference the project wiki under Notes/wiki as needed.`,
  ].join("\n");
}

/** Snapshot walkthroughs state before the agent runs. */
interface WalkthroughSnapshot {
  topDirs: Set<string>;
  files: Set<string>;
}

async function snapshotWalkthroughs(): Promise<WalkthroughSnapshot> {
  return {
    topDirs: new Set(await topSubdirs(WALKTHROUGHS_DIR)),
    files: await listFilesDeep(WALKTHROUGHS_DIR),
  };
}

/**
 * Verify the agent wrote a walkthrough: a new top-level subdirectory whose
 * name starts with the task prefix, containing at least one new file that is
 * at least ten lines long.
 */
async function verifyWalkthrough(
  prefix: string,
  before: WalkthroughSnapshot,
): Promise<{ ok: true; dir: string; file: string } | { ok: false; reason: string }> {
  const afterDirs = await topSubdirs(WALKTHROUGHS_DIR);
  const newDirs = afterDirs.filter((d) => d.startsWith(prefix) && !before.topDirs.has(d));
  if (newDirs.length === 0) {
    const existing = afterDirs.filter((d) => d.startsWith(prefix));
    return {
      ok: false,
      reason:
        existing.length === 0
          ? `no walkthrough directory starting with "${prefix}" was created`
          : `walkthrough directory "${existing[0]}" already existed before the agent ran (not new)`,
    };
  }
  const dir = newDirs[0];
  const afterFiles = await listFilesDeep(WALKTHROUGHS_DIR);
  const newFiles = [...afterFiles].filter((f) => !before.files.has(f) && f.startsWith(`${dir}/`));
  if (newFiles.length === 0) {
    return {
      ok: false,
      reason: `walkthrough directory "${dir}" has no new files`,
    };
  }
  for (const rel of newFiles) {
    const text = await readFile(join(WALKTHROUGHS_DIR, rel), "utf8");
    const lines = text.split("\n").filter((l) => l.trim().length > 0).length;
    if (lines >= 10) {
      return { ok: true, dir, file: rel };
    }
  }
  return {
    ok: false,
    reason: `walkthrough directory "${dir}" has new file(s) but none are at least ten lines long`,
  };
}

/** Send a notification via /home/chris/notify-app. */
async function notify(message: string): Promise<void> {
  if (DRY_RUN) {
    console.log(`[dry-run] would notify: ${NOTIFY_APP} "${message}"`);
    return;
  }
  try {
    await execFileP(NOTIFY_APP, [message]);
  } catch (err) {
    console.error(`notify-app failed: ${err}`);
  }
}

/** Run the pi agent session against the task prompt, streaming output live. */
async function runPi(prompt: string): Promise<void> {
  const createRuntime: CreateAgentSessionRuntimeFactory = async ({
    cwd,
    sessionManager,
    sessionStartEvent,
  }) => {
    const services = await createAgentSessionServices({ cwd });
    const resolved = resolveCliModel({ cliModel: PI_MODEL, modelRuntime: services.modelRuntime });
    if (resolved.error) throw new Error(`Could not resolve model "${PI_MODEL}": ${resolved.error}`);
    if (resolved.warning) console.warn(`Model warning: ${resolved.warning}`);
    return {
      ...(await createAgentSessionFromServices({
        services,
        sessionManager,
        sessionStartEvent,
        model: resolved.model,
        thinkingLevel: resolved.thinkingLevel,
      })),
      services,
      diagnostics: services.diagnostics,
    };
  };

  const runtime = await createAgentSessionRuntime(createRuntime, {
    cwd: ROOT,
    agentDir: getAgentDir(),
    sessionManager: SessionManager.create(ROOT),
  });

  const session = runtime.session;

  // Stream assistant text deltas to stdout as they happen.
  // Skip tool activity events and blank-only lines.
  let lineBuf = "";
  const unsubscribe = session.subscribe((event) => {
    if (event.type === "message_update") {
      const ame = event.assistantMessageEvent;
      if (ame.type === "text_delta") {
        lineBuf += ame.delta;
        let nl: number;
        while ((nl = lineBuf.indexOf("\n")) >= 0) {
          const line = lineBuf.slice(0, nl);
          lineBuf = lineBuf.slice(nl + 1);
          if (line.trim().length > 0) {
            process.stdout.write(line + "\n");
          }
        }
      }
    }
  });

  try {
    await session.prompt(prompt);
  } finally {
    unsubscribe();
    await runtime.dispose();
  }
}

/**
 * Run the full Go test suite (go test ./...) in the project root, matching
 * the walkthrough demo's verification step. Returns ok=false on a non-zero
 * exit, with the combined stdout/stderr for the notification.
 */
async function runTestSuite(): Promise<{ ok: true } | { ok: false; output: string }> {
  try {
    const { stdout, stderr } = await execFileP("go", ["test", "-count=1", "./..."], {
      cwd: ROOT,
      maxBuffer: 10 * 1024 * 1024,
    });
    return { ok: true, output: (stdout + stderr).toString().trim() } as { ok: true };
  } catch (err) {
    const e = err as { stdout?: Buffer; stderr?: Buffer; message: string };
    const output = [
      (e.stdout?.toString() ?? "").trim(),
      (e.stderr?.toString() ?? "").trim(),
    ].filter(Boolean).join("\n");
    return { ok: false, output: output || e.message };
  }
}

async function main() {
  for (;;) {
    // Check for a "stop" file in the project root to break the loop early.
    try {
      await stat(STOP_FILE);
      log("Stop file detected; breaking out of loop.");
      break;
    } catch {
      // No stop file — continue.
    }

    const task = await nextTask();
    if (!task) {
      log("All tasks are implemented. Nothing to do.");
      return;
    }

    const taskPath = join(TASKS_DIR, task.file);
    const taskText = await readFile(taskPath, "utf8");
    const prompt = buildPrompt(task, taskText);

    log(`Next task: ${task.file} (prefix ${task.prefix})`);
    log(`Task file: ${taskPath}`);
    log(`Model: ${PI_MODEL}`);
    log(`Skills dir: ${SKILLS_CODE_WRITING_DIR}`);
    log(`Prompt (${prompt.length} chars):`);
    console.log(prompt);
    log("---");

    if (DRY_RUN) {
      log("[dry-run] stopping here; would invoke pi agent session next.");
      log("[dry-run] after pi, would run: go test ./...");
      log("[dry-run] after tests, would verify a new walkthrough dir/file >=10 lines.");
      log(`[dry-run] on success: jj describe -m "Task ${task.name} implemented by ${PI_MODEL_NAME}." && jj new`);
      log("[dry-run] on failure: /home/chris/notify-app \"<message>\"");
      return;
    }

    const before = await snapshotWalkthroughs();

    log("Invoking pi agent session...");
    try {
      await runPi(prompt);
    } catch (err) {
      const message = `pi agent session failed for task ${task.file}: ${err}`;
      log(message);
      await notify(message);
      break;
    }
    log("pi agent session finished.");
    log("---");

    log("Running all tests: go test ./...");
    const testResult = await runTestSuite();
    if (!testResult.ok) {
      const message = `tests failed for task ${task.file}:\n${testResult.output}`;
      log(message);
      await notify(message);
      break;
    }
    log("All tests passed.");
    log("---");

    const result = await verifyWalkthrough(task.prefix, before);
    if (!result.ok) {
      const message = `pi agent did not write a walkthrough for task ${task.file}: ${result.reason}`;
      log(message);
      await notify(message);
      break;
    }

    log(`Walkthrough verified: ${result.dir}/${result.file}`);

    const describeMsg = `Task ${task.name} implemented by ${PI_MODEL_NAME}.`;
    log(`Running: jj describe -m "${describeMsg}"`);
    await run("jj", ["describe", "-m", describeMsg]);
    log("Running: jj new");
    await run("jj", ["new"]);
    log(`Task ${task.name} done.`);
    log("===");

    if (REVIEW_RE.test(taskText)) {
      const message = `Reached REVIEW task ${task.file}; stopping loop for human review.`;
      log(message);
      await notify(message);
      break;
    }
  }
  log("Done.");
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
