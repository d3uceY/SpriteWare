--------------------------------------------------------------------------------
-- SpriteWare headless host.
--
--   aseprite -b --script-param root=<package> --script headless/runner.lua
--
-- One long-lived batch process answering the same commands as the live
-- extension. Documents stay in memory between commands, with active sprite,
-- layer, frame and undo history intact, and nothing hits disk unless written.
--
-- It loads the real extension rather than a copy of the handlers, so there is
-- one command table and the modes cannot drift.
--------------------------------------------------------------------------------

local root = app.params["root"] or "."
dofile(app.fs.joinPath(root, "extension", "spriteware.lua"))

local A = _G.SPRITEWARE

-- Every reply line starts with this marker: Aseprite and any `print` share
-- stdout, and a stray warning parsed as a reply answers the wrong call.
local MARK = "\30SW1 "

local function emit(payload)
  io.stdout:write(MARK .. A.encodeJson(payload) .. "\n")
  -- A pipe is block-buffered: without this flush every call times out.
  io.stdout:flush()
end

emit({
  type = "hello",
  protocol = A.protocol,
  extensionVersion = A.version,
  asepriteVersion = tostring(app.version),
  headless = true,
  features = A.features,
})

-- One line per command keeps the process and its documents alive. EOF means the
-- server closed stdin.
for line in io.stdin:lines() do
  if line ~= "" then
    local decodedOk, decoded = pcall(json.decode, line)
    if decodedOk and decoded then
      local frame = A.toPlain(decoded)
      if type(frame) == "table" and frame.id ~= nil then
        local answered, reply = pcall(A.handleCommand, frame)
        if not answered then
          reply = {
            id = frame.id,
            ok = false,
            error = {
              code = "lua_error",
              message = tostring(reply),
              details = { command = frame.cmd },
            },
          }
        end
        emit(reply)
      end
    end
  end
end
