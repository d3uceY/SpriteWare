// Live frontend loop: elm make on save + Vite.
//
// Elm has no watch mode of its own, and public/elm.js is a static file Vite
// serves as-is, so a saved .elm would never reach the running window. This
// recompiles the Elm entry point on every change under elm/src and lets Vite's
// full reload pick the new bundle up. Styles and modules in src/ are Vite's
// own problem.
//
// Started by `npm run dev`, which `wails3 dev` runs as its frontend process:
// the Go app stays up, the window reloads.

import { spawn } from "node:child_process";
import { watch } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

const children = [];
let building = false;
let again = false;
let timer = null;

function elmMake() {
  if (building) {
    again = true;
    return;
  }

  building = true;
  const build = spawn("elm make elm/src/Main.elm --output=public/elm.js", {
    cwd: root,
    shell: true,
    stdio: "inherit",
  });

  build.on("exit", () => {
    building = false;
    if (again) {
      again = false;
      elmMake();
    }
  });
}

function onElmChange() {
  // Editors write a file in several steps; one build per burst is enough.
  clearTimeout(timer);
  timer = setTimeout(elmMake, 120);
}

function shutdown(code) {
  for (const child of children) child.kill();
  process.exit(code);
}

process.on("SIGINT", () => shutdown(0));
process.on("SIGTERM", () => shutdown(0));

elmMake();

try {
  const watcher = watch(path.join(root, "elm", "src"), { recursive: true }, (event, file) => {
    if (file && file.endsWith(".elm")) onElmChange();
  });
  watcher.on("error", (err) => console.warn("[elm] watcher:", err.message));
} catch (err) {
  console.warn("[elm] cannot watch elm/src, Elm will not rebuild:", err.message);
}

// Forward the port flags `wails3 dev` passes through.
const vite = spawn(
  process.execPath,
  [path.join(root, "node_modules", "vite", "bin", "vite.js"), ...process.argv.slice(2)],
  { cwd: root, stdio: "inherit" },
);
children.push(vite);
vite.on("exit", (code) => shutdown(code ?? 0));
