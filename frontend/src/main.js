// The only file that knows about both worlds.
//
//   Elm  --port-->  main.js  --binding-->  Go
//   Elm  <--port--  main.js  <--promise--  Go
//
// Elm speaks plain JSON records. The bindings speak promises and generated
// model classes. Every conversion between the two happens here and nowhere
// else, which is why Elm never needs to know that a Promise exists.

import { Events } from "@wailsio/runtime";

import { App } from "../bindings/github.com/d3uceY/spriteware/backend/index.js";
import { Tick } from "../bindings/events/index.js";

// One entry per action Elm can send through the `request` port.
// `toPort` turns the Go result into the JSON shape the Elm port declares, so
// adding a binding is a one-line change here plus a new port in Main.elm.
const actions = {
  info: {
    call: () => App.Info(),
    port: "infoFromJs",
    toPort: (info) => ({
      version: info.version,
      go: info.go,
      wails: info.wails,
      platform: info.platform,
    }),
  },
  echo: {
    call: (text) => App.Echo(text),
    port: "replyFromJs",
    toPort: (reply) => String(reply),
  },
};

const elm = window.Elm.Main.init({
  node: document.getElementById("app"),
  flags: null,
});

elm.ports.request.subscribe(async ({ action, target }) => {
  const handler = actions[action];
  if (!handler) {
    elm.ports.errorFromJs.send(`unknown action "${action}"`);
    return;
  }

  try {
    const result = await handler.call(target);
    elm.ports[handler.port].send(handler.toPort(result));
  } catch (err) {
    // Elm models success or failure, never an exception - so everything that
    // throws becomes a string on the error port.
    elm.ports.errorFromJs.send(describe(err));
  }
});

// The push direction: Go emits "tick", the runtime hands it to us, Elm gets a
// subscription. There is no Wails runtime in a plain browser preview, so a
// missing one is a warning rather than a crash.
try {
  Events.On(Tick, (event) => elm.ports.tickFromJs.send(String(event.data)));
} catch (err) {
  console.warn("[wails] custom events unavailable:", describe(err));
}

// Binding failures arrive as bare objects in the browser preview, so this has
// to cope with more than Error instances.
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
