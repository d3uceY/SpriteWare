// The only file that knows about both worlds.
//
//   Elm  --port-->  main.js  --binding-->  Go
//   Elm  <--port--  main.js  <--promise--  Go
//
// Elm owns state and structure. This file owns what it is bad at: talking to Go,
// flattening generated classes, and motion.

import { Events } from "@wailsio/runtime";
import gsap from "gsap";

// style.css lives in public/, which Vite copies verbatim - a bare @import there
// is a URL the browser cannot resolve, so the fonts are imported here instead
// and bundled with the app.
import "@fontsource-variable/albert-sans";
import "@fontsource-variable/pixelify-sans";
import "@fontsource/ibm-plex-mono/400.css";
import "@fontsource/ibm-plex-mono/500.css";

import { AppService, MCPService } from "../bindings/github.com/d3uceY/spriteware/backend/index.js";
import { Config } from "../bindings/github.com/d3uceY/spriteware/internal/config/models.js";

const STATUS_EVENT = "mcp:status";
const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

// An art pixel has to land on a whole number of device pixels. At 125% Windows
// scaling 2px is 2.5 device pixels, so the mascot's 1px eyes round to 2px wide
// on one side and 3px on the other. Snap --px to the nearest integer device
// pixel instead and hand it to CSS as --mark-px.
const dpr = window.devicePixelRatio || 1;
document.documentElement.style.setProperty(
  "--mark-px",
  `${Math.max(1, Math.round(2 * dpr)) / dpr}px`,
);

const elm = window.Elm.Main.init({
  node: document.getElementById("app"),
  flags: null,
});

// ---------------------------------------------------------------------------
// Actions
//
// One entry per action Elm sends through the `request` port. `run` does the work;
// `send` is the only place a port name is written down.
// ---------------------------------------------------------------------------

function send(port, value) {
  elm.ports[port].send(value);
}

async function refreshAll() {
  const [status, setup, tools, clients, skillText, project, settings] = await Promise.all([
    MCPService.GetMCPStatus(),
    MCPService.GetSetupState(),
    MCPService.ListTools(),
    MCPService.ListClients(),
    MCPService.SkillText(),
    MCPService.ProjectFolder(),
    MCPService.GetSettings(),
  ]);
  send("statusFromJs", toStatus(status));
  send("setupFromJs", toSetup(setup));
  send("toolsFromJs", (tools ?? []).map(toTool));
  send("clientsFromJs", (clients ?? []).map(toClient));
  send("skillTextFromJs", String(skillText ?? ""));
  send("projectFromJs", toProject(project));
  send("settingsFromJs", toSettings(settings));
}

async function refreshServer() {
  const [status, setup, clients] = await Promise.all([
    MCPService.GetMCPStatus(),
    MCPService.GetSetupState(),
    MCPService.ListClients(),
  ]);
  send("statusFromJs", toStatus(status));
  send("setupFromJs", toSetup(setup));
  send("clientsFromJs", (clients ?? []).map(toClient));
}

// The folder picker changes the skill paths on every card, so a refresh that
// skips the client list would leave the old paths on screen.
async function refreshAfterProjectChange() {
  const [project, clients] = await Promise.all([
    MCPService.ProjectFolder(),
    MCPService.ListClients(),
  ]);
  send("projectFromJs", toProject(project));
  send("clientsFromJs", (clients ?? []).map(toClient));
}

const actions = {
  // The version comes from the binary, not from package.json, so a stamped
  // release build reports the tag it was cut from. Neither call may be the
  // reason the window looks broken, so both are allowed to fail quietly.
  bootstrap: async () => {
    await refreshAll();
    try {
      send("versionFromJs", String((await AppService.GetVersion()) ?? ""));
      send("updateFromJs", toUpdate(await AppService.CheckForUpdate()));
    } catch (err) {
      console.warn("[update] version check failed:", describe(err));
    }
  },

  status: refreshServer,

  start: async () => {
    const settings = await MCPService.GetSettings();
    await MCPService.StartServer(settings.port, settings.transport);
    await refreshServer();
  },

  stop: async () => {
    await MCPService.StopServer();
    await refreshServer();
  },

  mode: async (mode) => {
    send("statusFromJs", toStatus(await MCPService.SwitchMode(mode)));
  },

  "save-settings": async (target) => {
    const [port, transport, bridgePort, on] = String(target ?? "").split("|");
    let current = null;
    try {
      current = await MCPService.GetSettings();
      const saved = await MCPService.SaveSettings(
        Config.createFrom({
          ...current,
          port: Number(port),
          transport: String(transport),
          bridgePort: Number(bridgePort),
          startMcpOnLaunch: on === "1",
        }),
      );
      send("savedSettingsFromJs", {
        ok: true,
        message: `Saved. Port ${Number(saved?.port ?? 0)}.`,
        settings: toSettings(saved),
      });
      // The listener may have moved, so the Server tab's address has to follow.
      await refreshServer();
    } catch (err) {
      // The note belongs on the card, next to the numbers that caused it.
      send("savedSettingsFromJs", {
        ok: false,
        message: describe(err),
        settings: toSettings(current),
      });
    }
  },

  "confirm-path": async (path) => {
    const setup = await MCPService.ConfirmAsepritePath(String(path ?? ""));
    send("setupFromJs", toSetup(setup));
    await refreshServer();
  },

  // The OS picker is the one way to name a file the user can be sure they
  // meant: a cancel comes back empty and changes nothing.
  "browse-aseprite": async () => {
    const setup = await MCPService.BrowseAsepritePath();
    send("setupFromJs", toSetup(setup));
    await refreshServer();
  },

  // The deep search can walk a while, so the button it came from has already
  // switched to "Searching…" by the time this resolves.
  "find-aseprite": async () => {
    const setup = await MCPService.DetectAseprite();
    send("setupFromJs", toSetup(setup));
    await refreshServer();
  },

  "forget-path": async () => {
    const setup = await MCPService.ForgetAsepritePath();
    send("setupFromJs", toSetup(setup));
    await refreshServer();
  },

  // Install writes the entry into the assistant's config. The answer is always a
  // message, so a refusal has somewhere to explain itself.
  install: async (id) => {
    const result = await MCPService.InstallClient(String(id ?? ""));
    send("installFromJs", {
      ok: Boolean(result?.ok),
      message: String(result?.message ?? ""),
    });
  },

  // "This project" has to be a folder the user picked: launched from a shortcut,
  // the working directory is the install folder, and a skill written there is one
  // no agent reads. With nothing chosen there is no project path at all.
  "choose-project": async () => {
    await MCPService.ChooseProjectFolder();
    await refreshAfterProjectChange();
  },

  // A skill is a folder with a SKILL.md, so this copies into the skills
  // directory instead of merging a config.
  "install-skill": async (target) => {
    const [agent, scope] = String(target ?? "").split("|");
    const result = await MCPService.InstallSkill(agent ?? "", scope ?? "project");
    send("skillFromJs", {
      ok: Boolean(result?.ok),
      message: String(result?.message ?? ""),
    });
  },

  // The release page opens in the user's browser; the app itself never
  // installs anything, because an unsigned build cannot verify its own
  // replacement.
  "open-downloads": async (url) => {
    await AppService.OpenDownloadsPage(String(url ?? ""));
  },
};

elm.ports.request.subscribe(async ({ action, target }) => {
  const handler = actions[action];
  if (!handler) {
    elm.ports.errorFromJs.send(`unknown action "${action}"`);
    return;
  }

  try {
    await handler(target);
  } catch (err) {
    // Elm models success or failure, never an exception, so anything that throws
    // becomes a string on the error port.
    elm.ports.errorFromJs.send(describe(err));
  }
});

// A clipboard write needs a user gesture and Elm cannot make one, so the click
// arrives here.
elm.ports.copyToClipboard.subscribe(async (text) => {
  try {
    await navigator.clipboard.writeText(text);
    return;
  } catch {
    // WebView2 can refuse the async API; the old selection trick still works.
  }

  const holder = document.createElement("textarea");
  holder.value = text;
  holder.setAttribute("readonly", "");
  holder.style.position = "fixed";
  holder.style.opacity = "0";
  document.body.appendChild(holder);
  holder.select();
  try {
    document.execCommand("copy");
  } finally {
    document.body.removeChild(holder);
  }
});

// Go pushes status whenever anything changes, including when the background
// search finishes, so the setup screen never shows a stale "not found".
try {
  Events.On(STATUS_EVENT, () => {
    refreshServer().catch((err) => send("errorFromJs", describe(err)));
  });
} catch (err) {
  console.warn("[wails] custom events unavailable:", describe(err));
}

// ---------------------------------------------------------------------------
// Flattening
//
// Doing it here means a rename in Go shows up as one broken field, not a blank
// screen.
// ---------------------------------------------------------------------------

function toStatus(raw) {
  const s = raw ?? {};
  return {
    running: Boolean(s.running),
    port: Number(s.port ?? 0),
    url: String(s.url ?? ""),
    transport: String(s.transport ?? ""),
    mode: String(s.mode ?? ""),
    bridgeConnected: Boolean(s.bridgeConnected),
    toolCount: Number(s.toolCount ?? 0),
    lastError: String(s.lastError ?? ""),
  };
}

function toSetup(raw) {
  const s = raw ?? {};
  return {
    confirmed: Boolean(s.confirmed),
    path: String(s.path ?? ""),
    candidate: String(s.candidate ?? ""),
    found: Boolean(s.found),
    searching: Boolean(s.searching),
    searched: Array.isArray(s.searched) ? s.searched.map(String) : [],
    problem: String(s.problem ?? ""),
  };
}

function toTool(raw) {
  return { name: String(raw?.name ?? ""), blurb: String(raw?.blurb ?? "") };
}

function toUpdate(raw) {
  const u = raw ?? {};
  return {
    current: String(u.current ?? ""),
    latest: String(u.latest ?? ""),
    available: Boolean(u.available),
    url: String(u.url ?? ""),
    notes: String(u.notes ?? ""),
  };
}

// ProjectFolder resolves to a [path, chosen] pair; the pair is meaningless to
// Elm, so it becomes a record here.
function toProject(raw) {
  const [path, chosen] = Array.isArray(raw) ? raw : [];
  return { path: String(path ?? ""), chosen: Boolean(chosen) };
}

function toSettings(raw) {
  const s = raw ?? {};
  return {
    port: Number(s.port ?? 0),
    transport: String(s.transport ?? "streamable-http"),
    bridgePort: Number(s.bridgePort ?? 0),
    startMcpOnLaunch: Boolean(s.startMcpOnLaunch),
  };
}

function toClient(raw) {
  const c = raw ?? {};
  const target = c.target ?? {};
  const install = c.install ?? {};
  const skill = c.skill ?? {};
  return {
    id: String(c.id ?? ""),
    label: String(c.label ?? ""),
    place: String(c.where ?? ""),
    note: String(c.note ?? ""),
    snippet: String(c.snippet ?? ""),
    target: { path: String(target.path ?? ""), detected: Boolean(target.detected) },
    install: { can: Boolean(install.can), why: String(install.why ?? "") },
    skill: {
      available: Boolean(skill.available),
      project: String(skill.project ?? ""),
      global: String(skill.global ?? ""),
      note: String(skill.note ?? ""),
    },
  };
}

function describe(err) {
  if (err == null) return "unknown error";
  if (typeof err === "string") return err;
  if (typeof err.message === "string" && err.message !== "") return err.message;
  try {
    return JSON.stringify(err);
  } catch {
    return String(err);
  }
}

// ---------------------------------------------------------------------------
// Motion
//
// A view change prints the title a glyph at a time and sets the page's blocks
// down in a few hard steps. Sprites animate in CSS. Nothing here runs under
// reduced motion.
// ---------------------------------------------------------------------------

// Pixels do not tween: every move here is quantised to a handful of positions.
const STEP = "steps(3)";

function printTitle() {
  const glyphs = document.querySelectorAll(".page__title .page__char");
  if (glyphs.length === 0) return;

  gsap.fromTo(
    glyphs,
    { autoAlpha: 0 },
    {
      autoAlpha: 1,
      duration: 0.001,
      stagger: 0.04,
      clearProps: "visibility,opacity",
    },
  );
}

function enterPage() {
  const blocks = document.querySelectorAll(".page > *");
  if (blocks.length === 0) return;

  gsap.from(blocks, {
    y: 8,
    opacity: 0,
    duration: 0.24,
    stagger: 0.05,
    ease: STEP,
    clearProps: "transform,opacity",
  });

  printTitle();
}

// First paint: the chrome settles before the page does.
function enterChrome() {
  gsap.from([".topbar", ".tabstrip"], {
    y: -8,
    opacity: 0,
    duration: 0.3,
    stagger: 0.06,
    ease: STEP,
    clearProps: "transform,opacity",
  });

  enterPage();
}

// Elm re-renders the whole tree, so this runs only when the view changes. It
// reports whether it did, because a screen change is not a patch.
let currentView = null;

function syncView() {
  const root = document.querySelector("[data-view]");
  const view = root?.dataset.view ?? null;
  if (view === currentView) return false;

  const first = currentView === null;
  currentView = view;

  if (!reducedMotion) {
    if (first) {
      enterChrome();
    } else {
      enterPage();
    }
  }

  return true;
}

// A page taller than the window loses its scroll position when the stage's
// children are replaced, so remember it and put it back after the patch. A new
// screen is not a patch: that one starts at the top, the way a document does.
let stageScroll = 0;

document.addEventListener(
  "scroll",
  (event) => {
    const target = event.target;
    if (target instanceof Element && target.classList.contains("stage")) {
      stageScroll = target.scrollTop;
    }
  },
  true,
);

function restoreScroll(freshScreen) {
  const stage = document.querySelector(".stage");
  if (!stage) return;

  if (freshScreen) {
    stageScroll = 0;
    stage.scrollTop = 0;
    return;
  }

  if (Math.abs(stage.scrollTop - stageScroll) > 1) {
    stage.scrollTop = stageScroll;
  }
}

// A menu with no room below it opens upward, the way the platform's own popup
// did for the `<select>` this replaced: a list clipped by the window edge is
// worse than one that grows the other way. Room is measured from the trigger
// and the list's scroll height rather than its rendered rect, because flipping
// on the rendered rect would oscillate - each flip makes the other direction
// look correct on the next pass.
function placeMenu() {
  const trigger = document.querySelector(".menu__trigger");
  const list = document.querySelector(".menu__list");
  if (!trigger || !list) return;

  // The list is capped by its own max-height, so what it needs is the smaller
  // of its content and that cap. Measuring only scrollHeight flips a list that
  // already fits into an upward one that hangs off the top of the window.
  const cap = parseFloat(getComputedStyle(list).maxHeight);
  const need = Math.min(list.scrollHeight, Number.isFinite(cap) ? cap : Infinity) + 4;

  const room = window.innerHeight - trigger.getBoundingClientRect().bottom - 8;
  list.classList.toggle("menu__list--up", need > room);
}

// ---------------------------------------------------------------------------
// The confirm dialog
//
// Elm re-renders the tree on every message, so the dialog node is new each time
// it appears and must not re-animate each time it is *still* there. One flag
// says whether it was already open, and it is cleared when the node goes away.
// ---------------------------------------------------------------------------

let dialogOpen = false;

function closeDialog() {
  const wrap = document.querySelector("[data-dialog]");
  wrap?.querySelector("[data-dialog-cancel]")?.click();
}

function dialogFocusables(wrap) {
  return [...wrap.querySelectorAll("button, [href], input, select, textarea, [tabindex]")]
    .filter((el) => !el.disabled && el.tabIndex >= 0 && el.getClientRects().length > 0);
}

function onDialogKey(event) {
  if (event.key === "Escape") {
    event.preventDefault();
    closeDialog();
    return;
  }

  // A tab that walks out of a modal lands on controls the user cannot see, so
  // it wraps instead. Escape is the way out, not Tab.
  if (event.key !== "Tab") return;
  const wrap = document.querySelector("[data-dialog]");
  if (!wrap) return;

  const stops = dialogFocusables(wrap);
  if (stops.length === 0) return;

  const first = stops[0];
  const last = stops[stops.length - 1];
  const active = document.activeElement;

  if (!wrap.contains(active)) {
    event.preventDefault();
    first.focus();
    return;
  }
  if (!event.shiftKey && active === last) {
    event.preventDefault();
    first.focus();
  }
  if (event.shiftKey && active === first) {
    event.preventDefault();
    last.focus();
  }
}

function syncDialog() {
  const wrap = document.querySelector("[data-dialog]");
  const app = document.querySelector(".app");

  // Everything but the dialog is inert while it is up. The chrome has buttons
  // of its own, so marking only the stage would still let a Tab escape into the
  // tab strip behind the scrim.
  if (app) {
    for (const child of app.children) {
      if (child.hasAttribute("data-dialog")) continue;
      child.inert = Boolean(wrap);
    }
  }

  if (!wrap) {
    if (dialogOpen) document.removeEventListener("keydown", onDialogKey);
    dialogOpen = false;
    return;
  }

  if (!dialogOpen) {
    dialogOpen = true;
    document.addEventListener("keydown", onDialogKey);

    const plate = wrap.querySelector(".dialog");
    if (!reducedMotion && plate) {
      // Four hard steps rather than an ease: the plate lands the way a pixel
      // dialog box always has, instead of growing like a sheet of paper.
      gsap.fromTo(
        plate,
        { scale: 0.9, opacity: 0 },
        { scale: 1, opacity: 1, duration: 0.24, ease: "steps(4)", transformOrigin: "50% 50%" },
      );
    }
  }

  // Elm replaces the dialog's nodes on every message, which drops focus to the
  // body. Put it back whenever it is no longer inside the dialog - and not
  // otherwise, or an intentional Tab between the two buttons would be undone.
  if (!wrap.contains(document.activeElement)) {
    // Focus the answer, not the dismissal: this dialog exists to be agreed with.
    wrap.querySelector("[data-dialog-confirm]")?.focus();
  }
}

function boot() {
  // Elm swaps its whole rendered tree on every update, including the node that
  // was handed to `init`, so the thing to watch is the body: every patch shows
  // up there, and nowhere else is guaranteed to survive one.
  const observer = new MutationObserver(() =>
    requestAnimationFrame(() => {
      placeMenu();
      syncDialog();
      restoreScroll(syncView());
    }),
  );
  observer.observe(document.body, { childList: true, subtree: true });

  placeMenu();
  syncDialog();
  restoreScroll(syncView());
}

boot();
