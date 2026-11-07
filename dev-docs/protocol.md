# Protocol

Two things talk this protocol: the app, and the Lua running inside Aseprite.
Both ends are ours, so it is as small as it can be. One message is one line of
JSON. No WebSocket, no length prefixes.

## Connecting

The first line names the caller.

```
{"role":"extension"}     Aseprite dialling in
{"role":"client"}        the tools dialling in
```

On the bridge that is all the handshake there is. Over the hidden process there
is no handshake: the Lua is already running when the app gets its first line.

## Two directions, one marker

Replies from Aseprite start with a marker:

```
\x1eSW1 {"id":"c1::r7","ok":true,"data":{...}}\n
```

`\x1e` is ASCII record separator, a byte no program prints by accident. It
exists because in headless mode Aseprite's own warnings share stdout with our
answers. A line without the marker is not ours and gets dropped.

Commands do not need it. They only travel one way:

```
{"id":"r7","cmd":"draw.apply","args":{...}}\n
```

## One command, end to end

```
agent -> draw tool          arguments in, ops compiled
      -> {"id":"r7", ...}   written to the bridge
      -> {"id":"c1::r7"}    bridge adds the client slot, forwards to Aseprite
      -> {"id":"c1::r7"}    Lua answers, still carrying the slot
      -> {"id":"r7"}        bridge strips the slot, routes to that client
```

The slot matters. Two agents may both use `r1` at the same time; the bridge
prefixes the id per client so a reply can find its way home. There is a test for
exactly that case.

## Messages

| Type | Sent by | Fields |
|---|---|---|
| hello | Lua, once per connection | `protocol`, `extensionVersion`, `asepriteVersion`, `headless`, `features` |
| command | app | `id`, `cmd`, `args` |
| reply | Lua | `id`, `ok`, `data` or `error`, and `target` on an object result |
| bridge_state | bridge | `connected`, `version`, `aseprite`, `clients` |

Every successful reply whose data is an object also carries `target`: the
document the command acted on, as `id`, `path`, `name` and `active`. Acting on
the wrong document and acting on the right one used to return the same shape,
which is how four exports came out as the wrong sprite. Array results
(`doc.list`) have no target, because a field on an array breaks its decode.

## Errors

A failed reply carries a code, a message and sometimes a fix:

```json
{"ok":false,"error":{"code":"not_connected",
 "message":"Aseprite is not connected to the bridge",
 "details":{},"remediation":"open Aseprite ... or switch to headless mode"}}
```

Codes: `not_connected`, `bridge_unavailable`, `headless_exited`, `invalid_args`,
`layer_not_found`, `frame_out_of_bounds`, `unsupported`, `not_found`,
`bad_config`, `timeout`, `internal`.

Agents branch on the code. The remediation is written for a human to read, and
the tool passes it through.

## Rules worth keeping

- A command that cannot be answered fails immediately with a reason. It never
  hangs, because a hang reads to an agent as "slow" and the retry is usually
  worse than the failure.
- Every command answers with its own id, so replies can arrive out of order.
- The Lua catches its own errors and turns them into `lua_error` replies, so one
  bad command cannot kill the session and the open documents with it.
