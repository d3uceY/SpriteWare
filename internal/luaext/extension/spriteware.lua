--------------------------------------------------------------------------------
-- SpriteWare — the Aseprite side.
--
-- One file. It works two ways:
--
--   live     loaded as an Aseprite extension; speaks over a websocket
--   headless dofile'd by headless/runner.lua inside `aseprite -b`, over stdin
--
-- Both modes run this exact command table, so the two can never drift apart.
--
-- Handlers return data, never text; the dispatcher builds the envelope, so a
-- tool cannot return an unparseable shape by accident.
--------------------------------------------------------------------------------

local PROTOCOL_VERSION = 1
local EXTENSION_VERSION = "0.2.0"

-- What this build can do. The server checks these before an op, so an old copy
-- fails loudly.
local FEATURES = {
  "dump", "draw_batch", "cel_batch", "frame_batch", "tag_batch",
  "cel_reset", "frame_normalise", "palette_dump", "doc_duplicate",
  "layer_group", "layer_merge", "palette_snap", "select_mask", "slice",
  "doc_targets", "draw_region", "draw_remap", "draw_frame", "draw_shift",
}

local SPRITEWARE = {}
_G.SPRITEWARE = SPRITEWARE

--------------------------------------------------------------------------------
-- JSON
--
-- Aseprite's encoder writes an empty table as `{}`, but the far side validates
-- findings, files and cels as arrays, and the wrong shape turns a successful
-- edit into a decode error. So encode here and say which tables are arrays.
--------------------------------------------------------------------------------

local ARRAY = { __spritewareArray = true }

local function arr(t)
  t = t or {}
  return setmetatable(t, ARRAY)
end

local ESCAPES = {
  ['"'] = '\\"', ["\\"] = "\\\\", ["\b"] = "\\b", ["\f"] = "\\f",
  ["\n"] = "\\n", ["\r"] = "\\r", ["\t"] = "\\t",
}

local function encode_string(s)
  return '"' .. s:gsub('[%c"\\]', function(c)
    return ESCAPES[c] or string.format("\\u%04x", c:byte())
  end) .. '"'
end

local encode_value

local function is_array(t)
  if getmetatable(t) == ARRAY then return true end
  local n = 0
  for k in pairs(t) do
    if type(k) ~= "number" then return false end
    n = n + 1
  end
  -- An empty table is an object; use arr{} to mean an empty array.
  return n > 0
end

local function encode_table(t, depth)
  if depth > 24 then return "null" end
  if is_array(t) then
    local parts = {}
    for i = 1, #t do parts[i] = encode_value(t[i], depth + 1) end
    return "[" .. table.concat(parts, ",") .. "]"
  end
  local parts = {}
  for k, v in pairs(t) do
    local encoded = encode_value(v, depth + 1)
    if encoded ~= nil then
      parts[#parts + 1] = encode_string(tostring(k)) .. ":" .. encoded
    end
  end
  return "{" .. table.concat(parts, ",") .. "}"
end

encode_value = function(v, depth)
  local kind = type(v)
  if v == nil then return "null" end
  if kind == "boolean" then return tostring(v) end
  if kind == "string" then return encode_string(v) end
  if kind == "number" then
    if v ~= v or v == math.huge or v == -math.huge then return "null" end
    if v == math.floor(v) and math.abs(v) < 2 ^ 53 then return string.format("%d", v) end
    return string.format("%.4f", v)
  end
  if kind == "table" then return encode_table(v, depth or 0) end
  return encode_string(tostring(v))
end

local function encode_json(v) return encode_value(v, 0) end

-- Aseprite's json.decode returns userdata with a metatable, not tables: type
-- checks and lengths break in a way that looks like the command never arrived.
-- Normalise once, at the boundary.
local function to_plain(value)
  local kind = type(value)
  if kind ~= "table" and kind ~= "userdata" then return value end

  local out = {}
  local length = 0
  pcall(function() length = #value end)

  -- `[1]` is the discriminator, not `#`: an object also reports a length, which
  -- would read it as an array of nils.
  if length > 0 and value[1] ~= nil then
    for i = 1, length do out[i] = to_plain(value[i]) end
    return out
  end

  local ok = pcall(function()
    for k, v in pairs(value) do out[k] = to_plain(v) end
  end)
  if not ok then return value end
  return out
end

--------------------------------------------------------------------------------
-- Errors
--------------------------------------------------------------------------------

local function fail(code, message, details)
  error({ __spriteware = true, code = code, message = message, details = details }, 0)
end

--------------------------------------------------------------------------------
-- Sprite helpers
--------------------------------------------------------------------------------

-- current is the document when there is no window to ask. Headless sets it; live
-- leaves it nil so the user's open sprite wins.
local current = nil

-- Unsaved documents have no filename, so a name has to be remembered here.
--
-- Keyed by document id, not by the Sprite userdata. The weak table this used to
-- be lost the name whenever Lua handed back a different wrapper for the same
-- document: `sprite new` named a document, and `sprite list` then reported
-- "Sprite" for it because it reached the sprite through `app.sprites` instead
-- of through `current`.
local unsavedNames = {}

local function name_key(spr)
  return tostring(spr.id)
end

-- color_mode_name turns the enum into a word.
--
-- Aseprite reports colorMode as a plain number, so stripping a "ColorMode."
-- prefix never matched and every document reported "0". The names come from the
-- enum itself rather than a guessed numbering.
local COLOR_MODE_NAMES = {}
for _, key in ipairs({ "RGB", "GRAYSCALE", "INDEXED", "BITMAP" }) do
  local value = ColorMode and ColorMode[key]
  if value ~= nil then COLOR_MODE_NAMES[tonumber(value) or value] = key:lower() end
end

local function color_mode_name(mode)
  local named = COLOR_MODE_NAMES[tonumber(mode) or mode]
  if named then return named end
  return (tostring(mode):gsub("^ColorMode%.", "")):lower()
end

-- same_path compares two absolute paths the way a user reads them: one separator
-- and one case. Aseprite hands back backslashes on Windows and the callers send
-- forward slashes, and a mismatch here is a silent wrong-document bug.
local function same_path(a, b)
  if not a or not b then return false end
  local function norm(p)
    return (tostring(p):gsub("\\", "/")):lower()
  end
  return norm(a) == norm(b)
end

-- open_paths is referenced inside asprite below, before its definition, so it
-- is forward-declared here. Without this, asprite resolves it as a nil global
-- and any failed sprite lookup crashes with "attempt to call a nil value".
local open_paths

-- match_sprite resolves whatever a caller names a document by.
--
-- Four spellings reach the same sprite: its path, its id, "unsaved:<id>" and the
-- name a `sprite op=new` gave it. The name used to be missing, so the `id` that
-- `sprite new` hands back could not be passed to any other tool — every call
-- answered "No open sprite matches 'probe-audit'". The same gap made
-- doc.activate unable to reach an unsaved document, which reports no filename.
local function match_sprite(want)
  local candidates = {}
  if current then candidates[#candidates + 1] = current end
  if app.activeSprite then candidates[#candidates + 1] = app.activeSprite end
  for _, spr in ipairs(app.sprites) do candidates[#candidates + 1] = spr end

  for _, spr in ipairs(candidates) do
    local named = unsavedNames[name_key(spr)]
    if same_path(spr.filename, want)
      or tostring(spr.id) == want
      or ("unsaved:" .. tostring(spr.id)) == want
      or (named ~= nil and named == want) then
      return spr
    end
  end
  return nil
end

local function asprite(args)
  if args and args.sprite and args.sprite ~= "" then
    local want = tostring(args.sprite)
    local found = match_sprite(want)
    if found then return found end
    fail("not_found", "No open sprite matches '" .. want .. "'.", { open = open_paths() })
  end
  if current then return current end
  local spr = app.activeSprite
  if not spr then
    fail("not_connected", "No sprite is open.",
      { hint = "open a document, or call sprite op=new first" })
  end
  return spr
end

open_paths = function()
  local out = arr()
  for _, spr in ipairs(app.sprites) do out[#out + 1] = spr.filename or "" end
  return out
end

-- is_saved is not `filename ~= ""`: Aseprite names a brand new document
-- "Sprite", so a non-empty filename proves nothing. A real file has a separator
-- or an extension.
local function is_saved(spr)
  local f = spr.filename
  if f == nil or f == "" then return false end
  if f:find("[/\\]") then return true end
  return f:find("%.[%w]+$") ~= nil
end

local function sprite_summary(spr)
  local named = unsavedNames[name_key(spr)]
  local saved = is_saved(spr)
  return {
    id = saved and spr.filename or (named or ("unsaved:" .. tostring(spr.id))),
    name = saved and splitext_basename(spr.filename) or (named or spr.filename or "Untitled"),
    path = saved and spr.filename or "",
    width = spr.width,
    height = spr.height,
    colorMode = color_mode_name(spr.colorMode),
    saved = saved,
  }
end

function splitext_basename(path)
  local name = path:gsub("^.*[/\\]", "")
  return (name:gsub("%.aseprite$", ""):gsub("%.ase$", ""))
end

-- target_ref names the document a reply acted on. Without it, acting on the
-- wrong document and acting on the right one return the same shape.
local function target_ref(spr)
  if not spr then return nil end
  local summary = sprite_summary(spr)
  local active = false
  local ok, live = pcall(function() return app.activeSprite end)
  if ok and live == spr then active = true end
  if current == spr then active = true end
  return { id = summary.id, path = summary.path, name = summary.name, active = active }
end

-- find_layer walks groups breadth-first, full name first so a shallower layer
-- wins, then reads "group/child" paths longest prefix first: Aseprite allows "/"
-- inside a layer name, so both readings can be valid.
local function find_layer(spr, name)
  if not name or name == "" then return nil end

  local queue = {}
  for _, layer in ipairs(spr.layers) do queue[#queue + 1] = layer end
  local head = 1
  while head <= #queue do
    local layer = queue[head]
    head = head + 1
    if layer.name == name then return layer end
    if layer.isGroup then
      for _, child in ipairs(layer.layers) do queue[#queue + 1] = child end
    end
  end

  if not name:find("/", 1, true) then return nil end
  local segs = {}
  for seg in name:gmatch("[^/]+") do segs[#segs + 1] = seg end

  local function resolve(layers, i)
    for j = #segs, i, -1 do
      local candidate = table.concat(segs, "/", i, j)
      for _, layer in ipairs(layers) do
        if layer.name == candidate then
          if j == #segs then return layer end
          if layer.isGroup then
            local found = resolve(layer.layers, j + 1)
            if found then return found end
          end
        end
      end
    end
    return nil
  end

  return resolve(spr.layers, 1)
end

local function require_layer(spr, name)
  local layer = find_layer(spr, name)
  if not layer then
    local names = arr()
    for _, l in ipairs(spr.layers) do names[#names + 1] = l.name end
    fail("layer_not_found", "No layer named '" .. tostring(name) .. "'.", { layers = names })
  end
  return layer
end

-- activate_layer selects a layer the only way Aseprite allows.
--
-- Reading `spr.activeLayer` is fine, but assigning it raises "Cannot set field
-- activeLayer" from the sprite's generic __newindex, which turns layer.activate
-- and layer.reorder into a Lua stack trace. App.activeLayer is the writable one,
-- and it acts on the active document, so a target in another file is made active
-- first or the assignment would land in the wrong sprite.
local function activate_layer(spr, layer)
  local ok, live = pcall(function() return app.activeSprite end)
  if ok and live ~= spr then app.activeSprite = spr end
  app.activeLayer = layer
end

-- `spr.frame` raises instead of returning nil when nothing is showing, which is
-- always the case under `aseprite -b`. It is not a Sprite property at all in
-- 1.3, so the lookup failed in live mode too and every caller fell back to 1:
-- "use the active frame" silently meant frame 1 for every op that omits
-- `frame`. App.activeFrame is the real one, and frameNumber is its 1-based
-- number. Frame 1 stays the answer when there is no window to ask.
local function active_frame_number(spr)
  -- App.activeFrame belongs to the active document, so it is only this sprite's
  -- answer when this sprite is the active one.
  local okLive, live = pcall(function() return app.activeSprite end)
  if not okLive or live == nil or live ~= spr then return 1 end

  local ok, frame = pcall(function() return app.activeFrame end)
  if ok and frame ~= nil then
    local n = tonumber(frame.frameNumber)
    if n and n >= 1 then return math.floor(n) end
  end
  return 1
end

local function frame_index(spr, args)
  local n = tonumber(args and args.frame)
  if not n then n = active_frame_number(spr) end
  n = math.floor(n)
  if n < 1 then n = 1 end
  if n > #spr.frames then n = #spr.frames end
  return n
end

-- normalize_cel replaces a cel's image with a canvas-sized one at (0,0), so
-- sprite-absolute coordinates are absolute. Without it a draw at (10,10) lands
-- at (10,10) minus the cel offset.
local function normalize_cel(spr, layer, frame, create)
  local cel = layer:cel(frame)
  if not cel then
    if not create then return nil end
    local img = Image(spr.width, spr.height, spr.colorMode)
    return spr:newCel(layer, frame, img, Point(0, 0))
  end
  if cel.position.x == 0 and cel.position.y == 0
    and cel.image.width == spr.width and cel.image.height == spr.height then
    return cel
  end
  local img = Image(spr.width, spr.height, spr.colorMode)
  img:drawImage(cel.image, cel.position)
  cel.image = img
  cel.position = Point(0, 0)
  return cel
end

local function pset(img, x, y, color)
  if x >= 0 and y >= 0 and x < img.width and y < img.height then
    img:putPixel(x, y, color)
  end
end

-- Selection --------------------------------------------------------------

-- usable_selection returns the sprite's selection, or nil when there is none.
--
-- Reading `spr.selection` is protected: under `aseprite -b` there is no visible
-- mask and the property can raise where a Selection would be returned.
local function usable_selection(spr)
  local ok, selection = pcall(function() return spr.selection end)
  if not ok or selection == nil then return nil end
  local emptyOk, empty = pcall(function() return selection.isEmpty end)
  if emptyOk and empty then return nil end
  return selection
end

-- mask_test builds the predicate a draw checks before writing a pixel.
--
-- Only `contains` is used. `Selection:bounds()` reports an empty rectangle
-- whenever the mask is not being drawn on screen, which under `aseprite -b` is
-- always, so a bounds check would reject every pixel of a real selection.
local function mask_test(selection)
  if selection == nil then
    return function() return true end
  end
  return function(x, y) return selection:contains(Point(x, y)) end
end

-- selection_area measures the mask by scanning the canvas. `select.info` wants
-- the rectangle and the pixel count, and both have to be measured for the same
-- reason bounds cannot be trusted.
local function selection_area(spr, selection)
  local minX, minY, maxX, maxY, pixels = math.huge, math.huge, -1, -1, 0
  for y = 0, spr.height - 1 do
    for x = 0, spr.width - 1 do
      if selection:contains(Point(x, y)) then
        pixels = pixels + 1
        if x < minX then minX = x end
        if y < minY then minY = y end
        if x > maxX then maxX = x end
        if y > maxY then maxY = y end
      end
    end
  end
  if maxX < 0 then return { empty = true, x = 0, y = 0, width = 0, height = 0, pixels = 0 } end
  return {
    empty = false,
    x = minX, y = minY,
    width = maxX - minX + 1, height = maxY - minY + 1,
    pixels = pixels,
  }
end

local function hex_components(hex)
  local s = tostring(hex or ""):gsub("#", "")
  if #s ~= 6 and #s ~= 8 then
    fail("invalid_args", "Colour '" .. tostring(hex) .. "' is not #rrggbb or #rrggbbaa.")
  end
  local r = tonumber(s:sub(1, 2), 16)
  local g = tonumber(s:sub(3, 4), 16)
  local b = tonumber(s:sub(5, 6), 16)
  local a = #s == 8 and tonumber(s:sub(7, 8), 16) or 255
  if not r or not g or not b or not a then
    fail("invalid_args", "Colour '" .. tostring(hex) .. "' is not valid hex.")
  end
  return r, g, b, a
end

-- color_value returns the integer a putPixel expects.
--
-- Indexed documents cannot store an arbitrary colour, so the nearest entry is
-- used and the snap is reported rather than landing silently on index 0.
local function color_value(spr, hex)
  if type(hex) == "number" then return hex end
  local r, g, b, a = hex_components(hex)

  if spr.colorMode == ColorMode.INDEXED then
    local pal = spr.palettes[1]
    if not pal then fail("invalid_args", "The document has no palette to index into.") end
    local best, bestDistance = 0, math.huge
    for i = 0, #pal - 1 do
      local c = pal:getColor(i)
      local dr, dg, db = c.red - r, c.green - g, c.blue - b
      local d = dr * dr + dg * dg + db * db
      if d < bestDistance then best, bestDistance = i, d end
    end
    return app.pixelColor.index(best)
  end

  return app.pixelColor.rgba(r, g, b, a)
end

local function color_hex(spr, pixel)
  local a = app.pixelColor.rgbaA(pixel)
  if a == 0 then return nil end
  local r = app.pixelColor.rgbaR(pixel)
  local g = app.pixelColor.rgbaG(pixel)
  local b = app.pixelColor.rgbaB(pixel)
  if a == 255 then return string.format("#%02x%02x%02x", r, g, b) end
  return string.format("#%02x%02x%02x%02x", r, g, b, a)
end

local function palette_hexes(spr)
  local out = arr()
  local pal = spr.palettes[1]
  if not pal then return out end
  for i = 0, #pal - 1 do
    local c = pal:getColor(i)
    out[#out + 1] = string.format("#%02x%02x%02x", c.red, c.green, c.blue)
  end
  return out
end

-- Blend modes -------------------------------------------------------------
--
-- The blend modes a layer reports are plain integers, and so is the `BlendMode`
-- table Aseprite exposes to Lua. Both use the compositing enum (MULTIPLY is 14,
-- not 1), which is why the numbers here come from `BlendMode` rather than from a
-- hand-written table: a wrong constant would silently set a different mode.
local BLEND_ENUM = {
  normal = "NORMAL",
  multiply = "MULTIPLY",
  screen = "SCREEN",
  overlay = "OVERLAY",
  darken = "DARKEN",
  lighten = "LIGHTEN",
  color_dodge = "COLOR_DODGE",
  color_burn = "COLOR_BURN",
  hard_light = "HARD_LIGHT",
  soft_light = "SOFT_LIGHT",
  difference = "DIFFERENCE",
  exclusion = "EXCLUSION",
  hue = "HUE",
  saturation = "SATURATION",
  color = "COLOR",
  luminosity = "LUMINOSITY",
  addition = "ADDITION",
  subtract = "SUBTRACT",
  divide = "DIVIDE",
}

local BLEND_SHORT = {}
for short, enumName in pairs(BLEND_ENUM) do
  local value = BlendMode[enumName]
  if value ~= nil then BLEND_SHORT[value] = short end
end

function blend_name(value)
  if value == nil then return "" end
  return BLEND_SHORT[tonumber(value)] or tostring(value)
end

function blend_value(short)
  local enumName = BLEND_ENUM[tostring(short):lower()]
  if enumName == nil then return nil end
  return BlendMode[enumName]
end

local function layer_entries(spr)
  local out = arr()
  local function walk(layers, depth)
    for _, layer in ipairs(layers) do
      local entry = {
        name = layer.name,
        depth = depth,
        isGroup = layer.isGroup,
        visible = layer.isVisible,
        opacity = layer.opacity,
        blendMode = blend_name(layer.blendMode),
        cels = arr(),
      }
      if not layer.isGroup then
        for _, cel in ipairs(layer.cels) do
          entry.cels[#entry.cels + 1] = {
            frame = cel.frameNumber,
            x = cel.position.x,
            y = cel.position.y,
            width = cel.image.width,
            height = cel.image.height,
          }
        end
      end
      out[#out + 1] = entry
      if layer.isGroup then walk(layer.layers, depth + 1) end
    end
  end
  walk(spr.layers, 0)
  return out
end

-- save_png writes one image to disk: Image:saveAs first, the sprite wrapper as
-- fallback. It returns the reason for a failure: a cel that never reaches disk
-- turns into a blank picture with nothing to debug.
local function save_png(img, path)
  local ok, routingErr = pcall(function() img:saveAs(path) end)
  if ok then return true end
  local spr = Sprite(img.width, img.height, img.colorMode)
  local cel = spr:newCel(spr.layers[1], 1, img:clone(), Point(0, 0))
  cel.image = img:clone()
  local ok2, spriteErr = pcall(function() spr:saveAs(path) end)
  pcall(function() spr:close() end)
  if ok2 then return true end
  return false, tostring(routingErr) .. " then " .. tostring(spriteErr)
end

--------------------------------------------------------------------------------
-- Handlers
--------------------------------------------------------------------------------

local H = {}

-- Session ---------------------------------------------------------------

-- ping is the cheapest liveness check: doctor calls it so reporting never
-- touches the user's document.
H["ping"] = function()
  return { pong = true, version = EXTENSION_VERSION }
end

H["doc.info"] = function(args)
  local spr = asprite(args)
  local summary = sprite_summary(spr)
  return {
    sprite = summary,
    layers = layer_entries(spr),
    frames = frame_entries(spr),
    tags = tag_entries(spr),
    slices = slice_entries(spr),
    palette = palette_hexes(spr),
    activeFrame = active_frame_number(spr),
    activeLayer = active_layer_index(spr),
    summary = string.format("%s %dx%d %s, %d frame(s), %d layer(s), %d colour(s)",
      summary.name, spr.width, spr.height,
      summary.colorMode, #spr.frames, #spr.layers, #spr.palettes[1]),
  }
end

H["doc.list"] = function(args)
  local out = arr()
  for _, spr in ipairs(app.sprites) do
    local entry = sprite_summary(spr)
    entry.open = (spr == app.activeSprite)
    out[#out + 1] = entry
  end
  return out
end

H["doc.new"] = function(args)
  local w = tonumber(args.width) or 32
  local h = tonumber(args.height) or 32
  if w < 1 or h < 1 or w > 4096 or h > 4096 then
    fail("invalid_args", "Canvas must be between 1x1 and 4096x4096.")
  end
  local frames = math.floor(tonumber(args.frames) or 1)
  if frames < 1 or frames > 512 then
    fail("invalid_args", "frames must be between 1 and 512.", { frames = frames })
  end
  local mode = ColorMode.RGB
  local spr = Sprite(w, h, mode)
  if #spr.layers == 0 or spr.layers[1] == nil then spr:newLayer() end
  spr.layers[1].name = "Layer 1"
  for i = 2, frames do spr:newEmptyFrame(i) end
  local name = tostring(args.name or "")
  if name ~= "" then unsavedNames[name_key(spr)] = name end
  current = spr
  app.activeSprite = spr
  return { sprite = sprite_summary(spr), frames = #spr.frames }
end

H["doc.open"] = function(args)
  local path = tostring(args.path or "")
  if path == "" then fail("invalid_args", "doc.open needs a path.") end

  -- Opening a path that is already open is a switch, not a second copy. Aseprite
  -- loads a new document here, which is how a caller ends up drawing into one
  -- copy while exporting the other.
  for _, spr in ipairs(app.sprites) do
    if same_path(spr.filename, path) then
      current = spr
      app.activeSprite = spr
      return { sprite = sprite_summary(spr), alreadyOpen = true, active = true }
    end
  end

  local spr = app.open(path)
  if not spr then
    local exists = app.fs.fileExists(path)
    if not exists then
      fail("not_found", "Aseprite could not find the file.", { path = path })
    end
    fail("open_failed", "Aseprite refused to open the file.", { path = path })
  end
  current = spr
  app.activeSprite = spr
  return { sprite = sprite_summary(spr), alreadyOpen = false, active = true }
end

H["doc.activate"] = function(args)
  -- `path` arrives already resolved to an absolute path; `sprite` is what the
  -- caller typed, which may be an id or the name from `sprite op=new`, so both
  -- get a turn. A path-only match answered "That document is not open" for a
  -- document that was.
  local spr = match_sprite(tostring(args.path or ""))
  if not spr and args.sprite ~= nil and tostring(args.sprite) ~= "" then
    spr = match_sprite(tostring(args.sprite))
  end
  if not spr then
    fail("not_found", "That document is not open.",
      { path = tostring(args.path or ""), open = open_paths() })
  end
  current = spr
  app.activeSprite = spr
  return { sprite = sprite_summary(spr), active = true }
end

H["doc.save"] = function(args)
  local spr = asprite(args)
  if not is_saved(spr) then
    fail("invalid_args", "This sprite has never been saved. Use sprite op=save_as with a path.")
  end
  spr:saveAs(spr.filename)
  return { path = spr.filename }
end

H["doc.saveAs"] = function(args)
  local spr = asprite(args)
  local path = tostring(args.path or "")
  if path == "" then fail("invalid_args", "doc.saveAs needs a path.") end
  spr:saveAs(path)
  return { path = path }
end

H["doc.duplicate"] = function(args)
  local spr = asprite(args)
  local path = tostring(args.path or "")
  if path == "" then fail("invalid_args", "doc.duplicate needs a destination path.") end
  spr:saveCopyAs(path)
  return { path = path }
end

H["doc.close"] = function(args)
  local spr = asprite(args)
  if not is_saved(spr) then
    fail("invalid_args", "Refusing to close a document that was never saved; it would be lost.")
  end
  spr:close()
  if current == spr then current = nil end
  unsavedNames[name_key(spr)] = nil
  return { closed = true }
end

-- doc.closeAll clears a long session. It refuses to drop unsaved work unless the
-- caller says force, so a sweep cannot silently lose a document.
H["doc.closeAll"] = function(args)
  local list = {}
  local unsaved = arr()
  for _, spr in ipairs(app.sprites) do
    list[#list + 1] = spr
    if not is_saved(spr) then unsaved[#unsaved + 1] = sprite_summary(spr).name end
  end
  if #unsaved > 0 and args.force ~= true then
    fail("conflict", "Refusing to close documents that were never saved.",
      { unsaved = unsaved, hint = "save them, or pass force=true to discard them" })
  end
  for _, spr in ipairs(list) do spr:close() end
  current = nil
  unsavedNames = {}
  return { closed = #list }
end

H["doc.resize"] = function(args)
  local spr = asprite(args)
  local w = tonumber(args.width) or spr.width
  local h = tonumber(args.height) or spr.height
  spr:resize(w, h)
  return { width = spr.width, height = spr.height }
end

-- doc.dump writes one PNG per cel plus the metadata Go reads. Go creates and
-- passes the directory.
H["doc.dump"] = function(args)
  local spr = asprite(args)
  local dir = tostring(args.dir or "")
  if dir == "" then fail("invalid_args", "doc.dump needs a scratch directory.") end
  -- Windows hands back backslashes. Sprite accepts them, Image:saveAs does not
  -- with a mixed path, so settle on one style before any cel path is built.
  dir = dir:gsub("\\", "/")

  local filter = args.filter or {}
  local wantLayer = {}
  if filter.layers and #filter.layers > 0 then
    for _, name in ipairs(filter.layers) do wantLayer[name] = true end
  end
  local wantFrame = {}
  if filter.frames and #filter.frames > 0 then
    for _, f in ipairs(filter.frames) do wantFrame[tonumber(f)] = true end
  end
  local function keepFrame(n)
    if next(wantFrame) == nil then return true end
    return wantFrame[n] == true
  end

  local layers = layer_entries(spr)
  -- Attach the actual pixels.
  local walkIndex = 0
  local saveFailed = nil
  local function attach(layersSrc, entries)
    for i, layer in ipairs(layersSrc) do
      local entry = entries[i]
      if layer.isGroup then
        attach(layer.layers, entry.layers or {})
      else
        local keepLayer = next(wantLayer) == nil or wantLayer[layer.name]
        if keepLayer then
          for ci, cel in ipairs(layer.cels) do
            if keepFrame(cel.frameNumber) then
              walkIndex = walkIndex + 1
              local name = string.format("cel_%d_%d.png", walkIndex, cel.frameNumber)
              local saved, why = save_png(cel.image, dir .. "/" .. name)
              if saved then
                for _, e in ipairs(entry.cels) do
                  if e.frame == cel.frameNumber then e.png = name end
                end
              elseif not saveFailed then
                saveFailed = why
              end
            end
          end
        end
      end
    end
  end
  attach(spr.layers, layers)

  if saveFailed then
    fail("internal", "Could not write the cel images to the scratch folder.", { error = saveFailed, dir = dir })
  end

  local payload = {
    sprite = sprite_summary(spr),
    layers = layers,
    frames = frame_entries(spr),
    tags = tag_entries(spr),
    slices = slice_entries(spr),
    palette = palette_hexes(spr),
    activeFrame = active_frame_number(spr),
    activeLayer = active_layer_index(spr),
    dir = dir,
  }

  -- Go reads the directory back as a document, so the metadata must be on disk
  -- beside the cel PNGs, not only in this reply.
  local manifest = io.open(dir .. "/dump.json", "wb")
  if not manifest then
    fail("internal", "Could not write the dump manifest.", { dir = dir })
  end
  manifest:write(encode_json(payload))
  manifest:close()

  return payload
end

function frame_entries(spr)
  local out = arr()
  for _, f in ipairs(spr.frames) do
    out[#out + 1] = { durationMs = math.floor(f.duration * 1000 + 0.5) }
  end
  return out
end

-- AniDir is an enum whose tostring is a number, so a caller reading the dump
-- would have to know the enum to tell forward from reverse. Everything else in
-- the model speaks names, so this does too.
local ANI_DIR_NAMES = { [0] = "forward", [1] = "reverse", [2] = "pingpong", [3] = "pingpong_reverse" }

function ani_dir_name(value)
  if value == nil then return "forward" end
  local n = tonumber(value)
  if n ~= nil then return ANI_DIR_NAMES[n] or "forward" end
  return tostring(value):gsub("^AniDir%.", ""):lower()
end

function tag_entries(spr)
  local out = arr()
  for _, tag in ipairs(spr.tags) do
    out[#out + 1] = {
      name = tag.name,
      from = tag.fromFrame.frameNumber,
      to = tag.toFrame.frameNumber,
      direction = ani_dir_name(tag.aniDir),
    }
  end
  return out
end

-- slice_entries reports every slice: the rectangle, plus the 9-patch centre
-- when one is set. A slice's bounds can change per frame in Aseprite; the first
-- key is the one an exporter needs, and that is what this reports.
function slice_entries(spr)
  local out = arr()
  for _, slice in ipairs(spr.slices) do
    local b = slice.bounds
    local entry = { name = slice.name, x = b.x, y = b.y, width = b.width, height = b.height }
    local center = slice.center
    if center and center.width > 0 and center.height > 0 then
      entry.center = { x = center.x, y = center.y, width = center.width, height = center.height }
    end
    out[#out + 1] = entry
  end
  return out
end

function active_layer_index(spr)
  local active = active_layer(spr)
  if active == nil then return 0 end
  for i, layer in ipairs(spr.layers) do
    if layer == active then return i - 1 end
  end
  return 0
end

-- A Sprite() has no active layer until Aseprite shows it, which never happens
-- under `aseprite -b`: reading `spr.activeLayer` raises instead of returning nil,
-- so the read is protected. Otherwise the first layer, never nil.
function active_layer(spr)
  local ok, layer = pcall(function() return spr.activeLayer end)
  if ok and layer ~= nil then return layer end
  return spr.layers[1]
end

function active_layer_name(spr)
  local layer = active_layer(spr)
  if layer == nil then return nil end
  return layer.name
end

-- Structure -------------------------------------------------------------

H["layer.list"] = function(args)
  return { layers = layer_entries(asprite(args)) }
end

H["layer.create"] = function(args)
  local spr = asprite(args)
  local name = tostring(args.name or "Layer")
  local layer = spr:newLayer()
  layer.name = name
  return { name = layer.name, index = #spr.layers - 1 }
end

H["layer.rename"] = function(args)
  local spr = asprite(args)
  local layer = require_layer(spr, args.name)
  layer.name = tostring(args.to or "")
  return { name = layer.name }
end

H["layer.delete"] = function(args)
  local spr = asprite(args)
  local layer = require_layer(spr, args.name)
  spr:deleteLayer(layer)
  return { deleted = true }
end

H["layer.set"] = function(args)
  local spr = asprite(args)
  local layer = require_layer(spr, args.name)
  if args.visible ~= nil then layer.isVisible = args.visible and true or false end
  if args.opacity ~= nil then layer.opacity = math.max(0, math.min(255, tonumber(args.opacity))) end
  if args.blendMode ~= nil then
    local value = blend_value(args.blendMode)
    if value == nil then
      local accepted = arr()
      for name in pairs(BLEND_ENUM) do accepted[#accepted + 1] = name end
      table.sort(accepted)
      fail("invalid_args", "Unknown blend mode '" .. tostring(args.blendMode) .. "'.",
        { modes = accepted })
    end
    layer.blendMode = value
  end
  return {
    name = layer.name,
    visible = layer.isVisible,
    opacity = layer.opacity,
    blendMode = blend_name(layer.blendMode),
  }
end

H["layer.duplicate"] = function(args)
  local spr = asprite(args)
  local source = require_layer(spr, args.name)
  local copy = spr:newLayer()
  copy.name = tostring(args.to or (source.name .. " copy"))
  for _, frame in ipairs(spr.frames) do
    local cel = source:cel(frame)
    if cel then
      local img = cel.image:clone()
      spr:newCel(copy, frame, img, cel.position)
    end
  end
  return { name = copy.name }
end

H["layer.activate"] = function(args)
  local spr = asprite(args)
  local layer = require_layer(spr, args.name)
  activate_layer(spr, layer)
  return { name = layer.name }
end

-- siblings_of returns a layer's siblings in stack order. A root layer's parent
-- is the Sprite, which has no `layers` list of its own, so root siblings are
-- the sprite's list.
local function siblings_of(spr, layer)
  local parent = layer.parent
  if parent == spr or not parent.isGroup then return spr.layers end
  return parent.layers
end

local function stack_index_of(spr, layer)
  for i, sibling in ipairs(siblings_of(spr, layer)) do
    if sibling == layer then return i end
  end
  return nil
end

-- reorder lives below siblings_of on purpose: a `local function` is only in
-- scope after its own line, so the same body above it called a nil global.
H["layer.reorder"] = function(args)
  local spr = asprite(args)
  local layer = require_layer(spr, args.name)

  local siblings = siblings_of(spr, layer)
  local target = tonumber(args.index) or 0
  target = math.max(0, math.min(#siblings - 1, math.floor(target)))

  -- Assigning stackIndex is the whole move. It used to drive
  -- app.command.MoveLayerUp/MoveLayerDown, which this build does not have, so
  -- every reorder failed with "This Aseprite build has no layer reordering
  -- command". stackIndex is 1-based and the tool's index is 0-based, the same
  -- convention layer.group relies on.
  layer.stackIndex = target + 1

  -- Report where it actually is, not where it was asked to go.
  local landed = (stack_index_of(spr, layer) or (target + 1)) - 1
  return { index = landed, name = layer.name }
end

-- resolve_sources turns a list of names into layers, refusing the two things a
-- merge or a group cannot do: take a group itself, or span two parents.
local function resolve_sources(spr, names, verb)
  if type(names) ~= "table" or #names == 0 then
    fail("invalid_args", "layer." .. verb .. " needs `layers`: the names to work on.")
  end

  local sources, seen, parent = {}, {}, nil
  for _, name in ipairs(names) do
    local layer = require_layer(spr, name)
    if not seen[layer] then
      seen[layer] = true
      if layer.isGroup then
        fail("invalid_args", "'" .. layer.name .. "' is a group; " .. verb .. " works on image layers.",
          { layers = names })
      end
      if parent == nil then
        parent = layer.parent
      elseif layer.parent ~= parent then
        fail("invalid_args",
          "Every layer passed to " .. verb .. " must share a parent; '" .. layer.name .. "' does not.",
          { layers = names })
      end
      sources[#sources + 1] = layer
    end
  end

  -- Bottom to top, whatever order the caller listed them in.
  local order = {}
  for i, sibling in ipairs(siblings_of(spr, sources[1])) do order[sibling] = i end
  table.sort(sources, function(a, b) return (order[a] or 0) < (order[b] or 0) end)
  return sources, parent
end

H["layer.group"] = function(args)
  local spr = asprite(args)
  local sources, parent = resolve_sources(spr, args.layers, "group")

  -- The group takes the slot of the lowest layer going in, so the stack does not
  -- jump when a group is made.
  local index = stack_index_of(spr, sources[1])
  local group = spr:newGroup()
  group.name = tostring(args.name or args.to or "Group")
  group.parent = parent
  group.stackIndex = index

  for _, layer in ipairs(sources) do layer.parent = group end
  return { name = group.name, layers = args.layers, index = index - 1 }
end

H["layer.ungroup"] = function(args)
  local spr = asprite(args)
  -- The tool description offers `layers` for group/ungroup/merge, but ungroup
  -- only ever takes one group. Accept either spelling; anything else reported
  -- "No layer named 'nil'", which names the argument rather than the mistake.
  local want = args.name
  if (want == nil or want == "") and type(args.layers) == "table" then want = args.layers[1] end
  if want == nil or want == "" then
    fail("invalid_args", "layer.ungroup needs the group to ungroup.",
      { hint = "pass `name`, or one entry in `layers`" })
  end
  local group = require_layer(spr, want)
  if not group.isGroup then
    fail("invalid_args", "'" .. group.name .. "' is not a group; ungroup only takes a group.",
      { name = group.name })
  end

  local parent = group.parent
  local groupName = group.name
  -- A slot counter, not a live index: each child inserted at `index` pushes the
  -- group and everything before it down, so the next child goes one further.
  local index = stack_index_of(spr, group)
  local names = arr()
  for _, child in ipairs(group.layers) do
    names[#names + 1] = child.name
    child.parent = parent
    child.stackIndex = index
    index = index + 1
  end
  -- Deleting the group invalidates its own handle, so every read of it has to
  -- happen above this line.
  spr:deleteLayer(group)

  return { name = groupName, layers = names }
end

H["layer.merge"] = function(args)
  local spr = asprite(args)
  local sources, parent = resolve_sources(spr, args.layers, "merge")
  if #sources < 2 then
    fail("invalid_args", "layer.merge needs at least two layers; one layer alone has nothing to merge with.",
      { layers = args.layers })
  end

  local index = stack_index_of(spr, sources[1])
  local merged = spr:newLayer()
  merged.name = tostring(args.to or args.name or sources[1].name)

  for _, frame in ipairs(spr.frames) do
    local img = Image(spr.width, spr.height, spr.colorMode)
    for _, layer in ipairs(sources) do
      local cel = layer:cel(frame)
      if cel then img:drawImage(cel.image, cel.position) end
    end
    spr:newCel(merged, frame, img, Point(0, 0))
  end

  for _, layer in ipairs(sources) do spr:deleteLayer(layer) end

  merged.parent = parent
  local siblings = siblings_of(spr, merged)
  merged.stackIndex = math.min(index, #siblings)
  return { name = merged.name, merged = #sources, layers = args.layers }
end

-- Selection --------------------------------------------------------------
--
-- A selection is document state, not a tool mode: once set, every draw is
-- clipped by it. That is what Aseprite itself does, and it makes a local edit
-- expressible in one sentence instead of a per-pixel list.

local function selection_payload(spr, selection)
  if selection == nil then
    return { empty = true, x = 0, y = 0, width = 0, height = 0, pixels = 0 }
  end
  return selection_area(spr, selection)
end

H["select.set"] = function(args)
  local spr = asprite(args)
  local x = math.floor(tonumber(args.x) or 0)
  local y = math.floor(tonumber(args.y) or 0)
  local w = math.floor(tonumber(args.width) or 0)
  local h = math.floor(tonumber(args.height) or 0)
  if w <= 0 or h <= 0 then
    fail("invalid_args", "select.set needs a width and height above zero.", { x = x, y = y, width = w, height = h })
  end

  local selection = Selection(Rectangle(x, y, w, h))
  spr.selection = selection
  return selection_payload(spr, selection)
end

H["select.all"] = function(args)
  local spr = asprite(args)
  local selection = Selection(Rectangle(0, 0, spr.width, spr.height))
  spr.selection = selection
  return selection_payload(spr, selection)
end

H["select.none"] = function(args)
  local spr = asprite(args)
  local selection = Selection(Rectangle(0, 0, 0, 0))
  pcall(function() selection:deselect() end)
  spr.selection = selection
  return selection_payload(spr, nil)
end

H["select.info"] = function(args)
  local spr = asprite(args)
  return selection_payload(spr, usable_selection(spr))
end

-- Frames ----------------------------------------------------------------

H["frame.list"] = function(args)
  local spr = asprite(args)
  local out = arr()
  for i = 1, #spr.frames do
    local f = spr.frames[i]
    local cels = arr()
    for _, layer in ipairs(spr.layers) do
      local cel = layer:cel(f)
      if cel then
        cels[#cels + 1] = {
          layer = layer.name,
          x = cel.position.x,
          y = cel.position.y,
          width = cel.image.width,
          height = cel.image.height,
        }
      end
    end
    out[#out + 1] = { index = i, durationMs = math.floor(f.duration * 1000 + 0.5), cels = cels }
  end
  -- `spr.frame` raises under `aseprite -b` when no frame is showing.
  return { frames = out, activeFrame = active_frame_number(spr) }
end

H["frame.add"] = function(args)
  local spr = asprite(args)
  local count = math.max(1, math.floor(tonumber(args.count) or 1))
  for _ = 1, count do spr:newEmptyFrame(#spr.frames + 1) end
  return { frames = #spr.frames }
end

H["frame.duplicate"] = function(args)
  local spr = asprite(args)
  local from = frame_index(spr, args)
  local before = {}
  for _, layer in ipairs(spr.layers) do
    local cel = layer:cel(spr.frames[from])
    if cel then before[layer.name] = { x = cel.position.x, y = cel.position.y } end
  end

  local frame = spr:newEmptyFrame(#spr.frames + 1)
  for _, layer in ipairs(spr.layers) do
    local cel = layer:cel(spr.frames[from])
    if cel then
      spr:newCel(layer, frame, cel.image:clone(), cel.position)
    end
  end

  -- Report the offsets the copy inherited: a hidden 1px bob carried into five
  -- animations is invisible until it is animated, and then it is in every frame.
  local carried = arr()
  for name, pos in pairs(before) do
    if pos.x ~= 0 or pos.y ~= 0 then
      carried[#carried + 1] = { layer = name, x = pos.x, y = pos.y }
    end
  end
  return { frames = #spr.frames, carriedOffsets = carried }
end

H["frame.delete"] = function(args)
  local spr = asprite(args)
  local n = frame_index(spr, args)
  if #spr.frames <= 1 then
    fail("invalid_args", "A sprite needs at least one frame.")
  end
  spr:deleteFrame(spr.frames[n])
  return { frames = #spr.frames, activeFrame = spr.frame }
end

H["frame.setDuration"] = function(args)
  local spr = asprite(args)
  local ms = tonumber(args.durationMs)
  if ms == nil then fail("invalid_args", "frame set_duration needs durationMs.") end
  local list = args.frames
  if list == nil or #list == 0 then
    for _, f in ipairs(spr.frames) do f.duration = ms / 1000 end
  else
    for _, n in ipairs(list) do
      local idx = math.floor(tonumber(n) or 0)
      if idx >= 1 and idx <= #spr.frames then spr.frames[idx].duration = ms / 1000 end
    end
  end
  return { durations = frame_entries(spr) }
end

H["frame.activate"] = function(args)
  local spr = asprite(args)
  local n = frame_index(spr, args)

  -- `spr.frame` is read-only here and raises "Cannot set field frame", so the
  -- frame has to be set on App. App_set_frame reads the argument with
  -- get_frame_number_from_arg, which takes a Frame object or a 1-based number
  -- and otherwise does lua_tointeger on it: spr.frames[n] is not always a
  -- FrameObj, and a table there became frame -1, which Aseprite clamped to
  -- frame 1. The number cannot be misread.
  local ok, live = pcall(function() return app.activeSprite end)
  if ok and live ~= spr then app.activeSprite = spr end
  app.activeFrame = n

  local reached = active_frame_number(spr)
  return { activeFrame = reached, requested = n, applied = reached == n }
end

H["frame.normalise"] = function(args)
  local spr = asprite(args)
  local n = frame_index(spr, args)
  local moved = 0
  for _, layer in ipairs(spr.layers) do
    local cel = layer:cel(spr.frames[n])
    if cel and (cel.position.x ~= 0 or cel.position.y ~= 0) then
      cel.position = Point(0, 0)
      moved = moved + 1
    end
  end
  return { normalised = moved, frame = n }
end

-- Cels ------------------------------------------------------------------

H["cel.list"] = function(args)
  return H["frame.list"](args)
end

H["cel.create"] = function(args)
  local spr = asprite(args)
  local layer = require_layer(spr, args.layer)
  local n = frame_index(spr, args)
  local img = Image(spr.width, spr.height, spr.colorMode)
  if spr:newCel(layer, spr.frames[n], img, Point(0, 0)) == nil then
    fail("internal", "Aseprite refused to create the cel.")
  end
  return { frame = n, layer = layer.name }
end

H["cel.clear"] = function(args)
  local spr = asprite(args)
  local layer = require_layer(spr, args.layer)
  local n = frame_index(spr, args)
  local cel = layer:cel(spr.frames[n])
  if cel then cel.image:clear() end
  return { cleared = true, frame = n }
end

H["cel.delete"] = function(args)
  local spr = asprite(args)
  local layer = require_layer(spr, args.layer)
  local n = frame_index(spr, args)
  local cel = layer:cel(spr.frames[n])
  if cel then spr:deleteCel(cel) end
  return { deleted = true, frame = n }
end

H["cel.move"] = function(args)
  local spr = asprite(args)
  local layer = require_layer(spr, args.layer)
  local n = frame_index(spr, args)
  local cel = layer:cel(spr.frames[n])
  if not cel then fail("invalid_args", "There is no cel on that layer and frame.") end
  local dx = tonumber(args.dx) or 0
  local dy = tonumber(args.dy) or 0
  cel.position = Point(cel.position.x + dx, cel.position.y + dy)
  return { x = cel.position.x, y = cel.position.y }
end

-- cel.copy makes a rig cheap: split a baked sprite by copying each part onto its
-- own layer.
H["cel.copy"] = function(args)
  local spr = asprite(args)
  local srcLayer = require_layer(spr, args.fromLayer or args.layer)
  local srcFrame = frame_index(spr, { frame = args.fromFrame or args.frame })
  local srcCel = srcLayer:cel(spr.frames[srcFrame])
  if not srcCel then
    fail("invalid_args", "There is no cel on '" .. srcLayer.name .. "' at frame " .. srcFrame .. " to copy.")
  end

  local dstLayer = require_layer(spr, args.layer)
  local dstFrame = frame_index(spr, args)
  local cel = normalize_cel(spr, dstLayer, spr.frames[dstFrame], true)

  -- Snapshot the source: copying a layer onto itself must read what was there,
  -- not what this call already wrote.
  local src = Image(srcCel.image.width, srcCel.image.height, spr.colorMode)
  src:drawImage(srcCel.image, Point(0, 0))

  local dx = tonumber(args.dx) or 0
  local dy = tonumber(args.dy) or 0
  cel.image:drawImage(src, Point(
    srcCel.position.x - cel.position.x + dx,
    srcCel.position.y - cel.position.y + dy))

  return {
    from = { layer = srcLayer.name, frame = srcFrame },
    to = { layer = dstLayer.name, frame = dstFrame },
    -- x/y is the cel's position, the same quantity cel.list, cel.move and
    -- cel.set report. It used to be the source's content origin, so a caller
    -- reading `copy.x` and passing it on landed somewhere else. contentX/Y names
    -- the other answer explicitly: where the source's pixels ended up.
    x = cel.position.x,
    y = cel.position.y,
    contentX = cel.position.x + srcCel.position.x + dx,
    contentY = cel.position.y + srcCel.position.y + dy,
  }
end

H["cel.set"] = function(args)  local spr = asprite(args)
  local layer = require_layer(spr, args.layer)
  local n = frame_index(spr, args)
  local cel = normalize_cel(spr, layer, spr.frames[n], true)
  if args.x ~= nil and args.y ~= nil then
    cel.position = Point(tonumber(args.x), tonumber(args.y))
  end
  if args.opacity ~= nil then
    cel.opacity = math.max(0, math.min(255, tonumber(args.opacity)))
  end
  return { x = cel.position.x, y = cel.position.y, opacity = cel.opacity or 255 }
end

-- reset is the other half of duplicate: a cel back at the origin, so a copied
-- frame is neutral.
H["cel.reset"] = function(args)
  local spr = asprite(args)
  local layer = require_layer(spr, args.layer)
  local n = frame_index(spr, args)
  local cel = layer:cel(spr.frames[n])
  if not cel then return { reset = false } end
  cel.position = Point(0, 0)
  return { reset = true, frame = n, layer = layer.name }
end

-- ease maps 0..1 to 0..1. Spacing is the easing: a slow start bunches the first
-- frames up, which reads as anticipation.
local function ease(name, t)
  name = tostring(name or "linear")
  if name == "ease_in" then return t * t end
  if name == "ease_out" then return 1 - (1 - t) * (1 - t) end
  if name == "ease_in_out" then
    if t < 0.5 then return 2 * t * t end
    local u = 1 - t
    return 1 - 2 * u * u
  end
  if name == "smoothstep" then return t * t * (3 - 2 * t) end
  return t
end

-- ensure_cel_like copies `source`'s pixels into frame `n`, so an in-between has
-- something to move: a tween in one call instead of one redraw per frame.
local function ensure_cel_like(spr, layer, n, source)
  local existing = layer:cel(spr.frames[n])
  if existing then return existing end

  local snapshot = Image(source.image.width, source.image.height, spr.colorMode)
  snapshot:drawImage(source.image, Point(0, 0))

  local img = Image(spr.width, spr.height, spr.colorMode)
  img:drawImage(snapshot, source.position)
  local cel = spr:newCel(layer, spr.frames[n], img, Point(0, 0))
  cel.opacity = source.opacity
  return cel
end

H["cel.tween"] = function(args)
  local spr = asprite(args)
  local layer = require_layer(spr, args.layer)

  local from = math.floor(tonumber(args.fromFrame) or 1)
  local to = math.floor(tonumber(args.toFrame) or 0)
  if to <= from then
    fail("invalid_args", "tween needs toFrame greater than fromFrame.", { fromFrame = from, toFrame = to })
  end
  if to > #spr.frames then
    fail("frame_out_of_bounds", "frame " .. to .. " does not exist; this sprite has " .. #spr.frames,
      { frames = #spr.frames })
  end

  local startCel = layer:cel(spr.frames[from])
  if not startCel then
    fail("invalid_args", "there is no cel on '" .. layer.name .. "' at frame " .. from .. " to tween from.")
  end

  local curve = tostring(args.easing or "linear")
  local touched = arr()

  for n = from + 1, to do
    local t = ease(curve, (n - from) / (to - from))
    local cel = ensure_cel_like(spr, layer, n, startCel)

    if tonumber(args.opacityFrom) or tonumber(args.opacityTo) then
      local a = tonumber(args.opacityFrom) or startCel.opacity
      local b = tonumber(args.opacityTo) or a
      cel.opacity = math.floor(a + (b - a) * t + 0.5)
    end

    local x0, y0 = tonumber(args.fromX), tonumber(args.fromY)
    local x1, y1 = tonumber(args.toX), tonumber(args.toY)
    if x0 or y0 or x1 or y1 then
      x0, y0, x1, y1 = x0 or 0, y0 or 0, x1 or x0 or 0, y1 or y0 or 0
      cel.position = Point(
        math.floor(x0 + (x1 - x0) * t + 0.5),
        math.floor(y0 + (y1 - y0) * t + 0.5))
    end

    touched[#touched + 1] = n
  end

  return { layer = layer.name, frames = touched, easing = curve }
end

H["cel.oscillate"] = function(args)
  local spr = asprite(args)
  local layer = require_layer(spr, args.layer)

  local from = math.floor(tonumber(args.fromFrame) or 1)
  local to = math.floor(tonumber(args.toFrame) or #spr.frames)
  if to < from then
    fail("invalid_args", "oscillate needs toFrame at least fromFrame.")
  end
  if to > #spr.frames then
    fail("frame_out_of_bounds", "frame " .. to .. " does not exist; this sprite has " .. #spr.frames)
  end

  local startCel = layer:cel(spr.frames[from])
  if not startCel then
    fail("invalid_args", "there is no cel on '" .. layer.name .. "' at frame " .. from .. " to oscillate.")
  end

  local ax = tonumber(args.amplitudeX) or 0
  local ay = tonumber(args.amplitudeY) or 0
  local period = math.max(1, tonumber(args.period) or (to - from + 1))
  local phase = tonumber(args.phase) or 0

  -- Rounding costs: a 1 px amplitude over a short period lands two frames on the
  -- same offset, which reads as a stutter.
  local touched = arr()
  for n = from, to do
    local cel = ensure_cel_like(spr, layer, n, startCel)
    local angle = 2 * math.pi * (((n - from) / period) + phase)
    cel.position = Point(
      math.floor(ax * math.sin(angle) + 0.5),
      math.floor(ay * math.sin(angle) + 0.5))
    touched[#touched + 1] = n
  end

  return { layer = layer.name, frames = touched, period = period }
end

-- transform.apply acts on one cel: the pixels are rearranged, not redrawn.
H["transform.apply"] = function(args)
  local spr = asprite(args)
  local ops = args.ops or {}
  if #ops == 0 then return { pixelsChanged = 0, opsApplied = 0 } end

  local changed = 0
  local done = arr()

  for i, op in ipairs(ops) do
    local t = op.target or {}
    local layer = require_layer(spr, t.layer or active_layer_name(spr))
    local n = frame_index(spr, { frame = t.frame })
    local cel = normalize_cel(spr, layer, spr.frames[n], false)
    if not cel then
      fail("invalid_args", "transform needs pixels to work on: '" .. layer.name .. "' frame " .. n .. " is empty.",
        { opIndex = i })
    end

    local img = cel.image
    local kind = op.kind

    if kind == "translate" then
      local dx, dy = tonumber(op.dx) or 0, tonumber(op.dy) or 0
      cel.position = Point(cel.position.x + dx, cel.position.y + dy)
      changed = changed + 1

    elseif kind == "flip" then
      local direction = tostring(op.direction or "horizontal")
      local src = Image(img.width, img.height, spr.colorMode)
      src:drawImage(img, Point(0, 0))
      for y = 0, img.height - 1 do
        for x = 0, img.width - 1 do
          local sx = direction == "vertical" and x or (img.width - 1 - x)
          local sy = direction == "vertical" and (img.height - 1 - y) or y
          img:putPixel(x, y, src:getPixel(sx, sy))
        end
      end
      changed = changed + img.width * img.height

    elseif kind == "rotate" then
      local angle = tonumber(op.angle) or 0
      if angle % 90 ~= 0 then
        fail("invalid_args", "rotate only turns by multiples of 90 degrees; " .. tostring(angle) ..
          " would resample and destroy pixels.", { opIndex = i, hint = "redraw for other angles" })
      end
      local quarter = (math.floor(angle / 90) % 4 + 4) % 4
      if quarter ~= 0 then
        local src = Image(img.width, img.height, spr.colorMode)
        src:drawImage(img, Point(0, 0))
        local rotated = (quarter == 2)
          and Image(src.width, src.height, spr.colorMode)
          or Image(src.height, src.width, spr.colorMode)

        for y = 0, src.height - 1 do
          for x = 0, src.width - 1 do
            local p = src:getPixel(x, y)
            if quarter == 1 then
              rotated:putPixel(src.height - 1 - y, x, p)
            elseif quarter == 2 then
              rotated:putPixel(src.width - 1 - x, src.height - 1 - y, p)
            else
              rotated:putPixel(y, src.width - 1 - x, p)
            end
          end
        end
        cel.image = rotated
        changed = changed + src.width * src.height
      end

    elseif kind == "scale" then
      local factor = math.floor(tonumber(op.factor) or 1)
      if factor < 1 then
        fail("invalid_args", "scale needs a whole-number factor of at least 1; downscaling resamples and loses pixels.",
          { opIndex = i })
      end
      if factor > 1 then
        local src = Image(img.width, img.height, spr.colorMode)
        src:drawImage(img, Point(0, 0))
        local scaled = Image(src.width * factor, src.height * factor, spr.colorMode)
        for y = 0, src.height - 1 do
          for x = 0, src.width - 1 do
            local p = src:getPixel(x, y)
            for dy = 0, factor - 1 do
              for dx = 0, factor - 1 do
                scaled:putPixel(x * factor + dx, y * factor + dy, p)
              end
            end
          end
        end
        cel.image = scaled
        changed = changed + src.width * src.height * factor * factor
      end

    elseif kind == "crop_to_content" then
      local minX, minY, maxX, maxY = math.huge, math.huge, -1, -1
      for y = 0, img.height - 1 do
        for x = 0, img.width - 1 do
          if app.pixelColor.rgbaA(img:getPixel(x, y)) > 0 then
            if x < minX then minX = x end
            if y < minY then minY = y end
            if x > maxX then maxX = x end
            if y > maxY then maxY = y end
          end
        end
      end
      if maxX < minX then
        fail("invalid_args", "the cel is empty, so there is nothing to crop to.", { opIndex = i })
      end
      local w, h = maxX - minX + 1, maxY - minY + 1
      if w ~= img.width or h ~= img.height or minX ~= 0 or minY ~= 0 then
        local cropped = Image(w, h, spr.colorMode)
        for y = 0, h - 1 do
          for x = 0, w - 1 do
            cropped:putPixel(x, y, img:getPixel(minX + x, minY + y))
          end
        end
        cel.image = cropped
        cel.position = Point(cel.position.x + minX, cel.position.y + minY)
        changed = changed + 1
      end

    else
      fail("invalid_args", "Unknown transform kind '" .. tostring(kind) .. "'.", { opIndex = i })
    end

    done[#done + 1] = { kind = kind, layer = layer.name, frame = n }
  end

  return { pixelsChanged = changed, opsApplied = #ops, applied = done }
end

-- Tags ------------------------------------------------------------------

H["tag.list"] = function(args)
  return { tags = tag_entries(asprite(args)) }
end

H["tag.create"] = function(args)
  local spr = asprite(args)
  local name = tostring(args.name or "tag")
  local from = math.max(1, math.floor(tonumber(args.from) or 1))
  local to = math.min(#spr.frames, math.floor(tonumber(args.to) or #spr.frames))
  if from > to then fail("invalid_args", "tag from must be at or before to.") end
  local tag = spr:newTag(from, to)
  tag.name = name
  if args.direction == "reverse" then tag.aniDir = AniDir.REVERSE
  elseif args.direction == "pingpong" then tag.aniDir = AniDir.PING_PONG end
  return { name = tag.name, from = from, to = to }
end

H["tag.update"] = function(args)
  local spr = asprite(args)
  local want = tostring(args.name or "")
  local tag
  for _, t in ipairs(spr.tags) do
    if t.name == want then tag = t; break end
  end
  if not tag then
    local names = arr()
    for _, t in ipairs(spr.tags) do names[#names + 1] = t.name end
    fail("invalid_args", "No tag named '" .. want .. "'.", { tags = names })
  end

  -- Rename needs its own field: `to` is the range end, so reusing it would
  -- silently move the tag.
  if args.rename ~= nil and tostring(args.rename) ~= "" then
    tag.name = tostring(args.rename)
  end
  -- Work out the range first: Aseprite normalises from > to, so checking after
  -- the assignment would accept an inverted range.
  local function frame_arg(value, fallback)
    if value == nil then return fallback end
    local n = math.max(1, math.floor(tonumber(value) or fallback))
    if n > #spr.frames then n = #spr.frames end
    return n
  end

  local fromN = frame_arg(args.from, tag.fromFrame.frameNumber)
  local toN = frame_arg(args.to, tag.toFrame.frameNumber)
  if fromN > toN then
    fail("invalid_args", "the tag would start after it ends.", { from = fromN, to = toN })
  end
  tag.fromFrame = spr.frames[fromN]
  tag.toFrame = spr.frames[toN]

  if args.direction == "reverse" then
    tag.aniDir = AniDir.REVERSE
  elseif args.direction == "pingpong" then
    tag.aniDir = AniDir.PING_PONG
  elseif args.direction == "forward" then
    tag.aniDir = AniDir.FORWARD
  end

  return { name = tag.name, from = fromN, to = toN,
    direction = ani_dir_name(tag.aniDir) }
end

H["tag.delete"] = function(args)
  local spr = asprite(args)
  for i, tag in ipairs(spr.tags) do
    if tag.name == args.name then
      spr:deleteTag(tag)
      return { deleted = true, index = i }
    end
  end
  fail("invalid_args", "No tag named '" .. tostring(args.name) .. "'.")
end

-- Slices ------------------------------------------------------------------
--
-- A slice names a rectangle of the canvas: the 9-patch region of a UI panel, or
-- a character's bounding box. Exporters read it, so an importer does not have to
-- guess where a panel's border ends.

local function find_slice(spr, name)
  for _, slice in ipairs(spr.slices) do
    if slice.name == name then return slice end
  end
  return nil
end

H["slice.list"] = function(args)
  return { slices = slice_entries(asprite(args)) }
end

H["slice.create"] = function(args)
  local spr = asprite(args)
  local name = tostring(args.name or "slice")
  local x = math.floor(tonumber(args.x) or 0)
  local y = math.floor(tonumber(args.y) or 0)
  local w = math.floor(tonumber(args.width) or 0)
  local h = math.floor(tonumber(args.height) or 0)
  if w <= 0 or h <= 0 then
    fail("invalid_args", "slice.create needs a width and height above zero.", { name = name })
  end
  if find_slice(spr, name) then
    fail("invalid_args", "A slice named '" .. name .. "' already exists.", { name = name, hint = "slice op=update moves it" })
  end

  local slice = spr:newSlice(Rectangle(x, y, w, h))
  slice.name = name
  if type(args.center) == "table" then
    local c = args.center
    local cw = math.floor(tonumber(c.width) or 0)
    local ch = math.floor(tonumber(c.height) or 0)
    if cw > 0 and ch > 0 then
      slice.center = Rectangle(math.floor(tonumber(c.x) or 0), math.floor(tonumber(c.y) or 0), cw, ch)
    end
  end
  return { name = name, x = x, y = y, width = w, height = h }
end

H["slice.update"] = function(args)
  local spr = asprite(args)
  local slice = find_slice(spr, tostring(args.name or ""))
  if not slice then
    fail("slice_not_found", "No slice named '" .. tostring(args.name) .. "'.",
      { slices = slice_entries(spr) })
  end

  local b = slice.bounds
  local x = math.floor(tonumber(args.x) or b.x)
  local y = math.floor(tonumber(args.y) or b.y)
  local w = math.floor(tonumber(args.width) or b.width)
  local h = math.floor(tonumber(args.height) or b.height)
  if w <= 0 or h <= 0 then
    fail("invalid_args", "slice.update needs a width and height above zero.", { name = slice.name })
  end
  slice.bounds = Rectangle(x, y, w, h)

  if type(args.center) == "table" then
    local c = args.center
    local cw = math.floor(tonumber(c.width) or 0)
    local ch = math.floor(tonumber(c.height) or 0)
    if cw > 0 and ch > 0 then
      slice.center = Rectangle(math.floor(tonumber(c.x) or 0), math.floor(tonumber(c.y) or 0), cw, ch)
    end
  end
  if type(args.rename) == "string" and args.rename ~= "" then slice.name = args.rename end
  return { name = slice.name, x = x, y = y, width = w, height = h }
end

H["slice.delete"] = function(args)
  local spr = asprite(args)
  local slice = find_slice(spr, tostring(args.name or ""))
  if not slice then
    fail("slice_not_found", "No slice named '" .. tostring(args.name) .. "'.",
      { slices = slice_entries(spr) })
  end
  spr:deleteSlice(slice)
  return { deleted = true, name = tostring(args.name) }
end

-- Drawing ---------------------------------------------------------------

-- region_of reads the rectangle an op is limited to: x/y/width/height, or the
-- x0/y0/x1/y1 corners. nil means the whole cel. Silent acceptance of a region
-- an op never reads is how a replace recolours more than it was asked to.
local function region_of(op)
  local x0, y0 = tonumber(op.x0), tonumber(op.y0)
  local x1, y1 = tonumber(op.x1), tonumber(op.y1)
  if x0 and y0 and x1 and y1 then
    return {
      x = math.min(x0, x1), y = math.min(y0, y1),
      w = math.abs(x1 - x0) + 1, h = math.abs(y1 - y0) + 1,
    }
  end
  local x, y = tonumber(op.x), tonumber(op.y)
  local w, h = tonumber(op.width), tonumber(op.height)
  if x and y and w and h and w > 0 and h > 0 then
    return { x = x, y = y, w = w, h = h }
  end
  return nil
end

local function in_region(region, x, y)
  if region == nil then return true end
  return x >= region.x and y >= region.y
    and x < region.x + region.w and y < region.y + region.h
end

H["draw.apply"] = function(args)
  local spr = asprite(args)
  local ops = args.ops or {}
  if #ops == 0 then return { pixelsChanged = 0, opsApplied = 0 } end

  -- A selection clips every draw, the way Aseprite's own tools behave.
  -- `ignoreSelection` opts out for a deliberate whole-cel edit.
  --
  -- Written as an if, not `x and nil or y`: that idiom falls through to the last
  -- operand when the middle one is nil, which is exactly the value meant here.
  local selection = nil
  if args.ignoreSelection ~= true then selection = usable_selection(spr) end
  local inside = mask_test(selection)

  -- write is the one place a pixel is counted. `pixelsChanged` is the pixels
  -- whose value actually differed, which is what proves an edit landed;
  -- `pixelsWritten` counts the in-bounds writes the mask allowed.
  local written, changed = 0, 0
  local function write(img, x, y, colour)
    if x < 0 or y < 0 or x >= img.width or y >= img.height then return end
    if not inside(x, y) then return end
    written = written + 1
    if img:getPixel(x, y) ~= colour then
      img:putPixel(x, y, colour)
      changed = changed + 1
    end
  end

  -- Resolve every target before touching anything, so a bad op in the middle of
  -- a batch cannot leave the sprite half-drawn.
  local targets = {}
  for i, op in ipairs(ops) do
    local t = op.target or {}
    local layer = require_layer(spr, t.layer or active_layer_name(spr))
    local n = frame_index(spr, { frame = t.frame })
    targets[i] = normalize_cel(spr, layer, spr.frames[n], true)
  end

  for i, op in ipairs(ops) do
    local cel = targets[i]
    local img = cel.image
    local kind = op.kind

    if kind == "clear" then
      local x, y = tonumber(op.x) or 0, tonumber(op.y) or 0
      local w, h = tonumber(op.width) or 0, tonumber(op.height) or 0
      for py = y, y + h - 1 do
        for px = x, x + w - 1 do
          write(img, px, py, app.pixelColor.rgba(0, 0, 0, 0))
        end
      end

    elseif kind == "pixels" then
      local colour = color_value(spr, op.color)
      for _, p in ipairs(op.points or {}) do
        local x, y = tonumber(p[1]), tonumber(p[2])
        if x and y then
          write(img, x, y, colour)
        end
      end

    elseif kind == "rect" then
      local x, y = tonumber(op.x) or 0, tonumber(op.y) or 0
      local w, h = tonumber(op.width) or 0, tonumber(op.height) or 0
      local colour = color_value(spr, op.color)
      local filled = op.fill ~= false
      for py = y, y + h - 1 do
        for px = x, x + w - 1 do
          local edge = (px == x or py == y or px == x + w - 1 or py == y + h - 1)
          if filled or edge then
            write(img, px, py, colour)
          end
        end
      end

    elseif kind == "line" then
      local x0, y0 = tonumber(op.x0) or 0, tonumber(op.y0) or 0
      local x1, y1 = tonumber(op.x1) or 0, tonumber(op.y1) or 0
      local colour = color_value(spr, op.color)
      local dx, dy = math.abs(x1 - x0), math.abs(y1 - y0)
      local sx = x0 < x1 and 1 or -1
      local sy = y0 < y1 and 1 or -1
      local err = dx - dy
      while true do
        write(img, x0, y0, colour)
        if x0 == x1 and y0 == y1 then break end
        local e2 = 2 * err
        if e2 > -dy then err = err - dy; x0 = x0 + sx end
        if e2 < dx then err = err + dx; y0 = y0 + sy end
      end

    elseif kind == "ellipse" then
      local cx = tonumber(op.x) or 0
      local cy = tonumber(op.y) or 0
      local rx = tonumber(op.radiusX) or 0
      local ry = tonumber(op.radiusY) or rx
      local colour = color_value(spr, op.color)
      for py = cy - ry, cy + ry do
        for px = cx - rx, cx + rx do
          local nx = (px - cx) / math.max(rx, 1)
          local ny = (py - cy) / math.max(ry, 1)
          local in_shape = nx * nx + ny * ny <= 1.0
          if op.fill ~= false and in_shape then
            write(img, px, py, colour)
          elseif op.fill == false and in_shape then
            local outsideNeighbour =
              ((px + 1 - cx) / math.max(rx, 1)) ^ 2 + ((py - cy) / math.max(ry, 1)) ^ 2 > 1.0 or
              ((px - 1 - cx) / math.max(rx, 1)) ^ 2 + ((py - cy) / math.max(ry, 1)) ^ 2 > 1.0 or
              ((px - cx) / math.max(rx, 1)) ^ 2 + ((py + 1 - cy) / math.max(ry, 1)) ^ 2 > 1.0 or
              ((px - cx) / math.max(rx, 1)) ^ 2 + ((py - 1 - cy) / math.max(ry, 1)) ^ 2 > 1.0
            if outsideNeighbour then
              write(img, px, py, colour)
            end
          end
        end
      end

    elseif kind == "replace" then
      local from = color_value(spr, op.from)
      local to = color_value(spr, op.to)
      local region = region_of(op)
      for py = 0, img.height - 1 do
        for px = 0, img.width - 1 do
          if img:getPixel(px, py) == from and in_region(region, px, py) and inside(px, py) then
            write(img, px, py, to)
          end
        end
      end

    elseif kind == "remap" then
      -- Recolouring a theme is a map, not a chain of replaces: a target that is
      -- also a source would otherwise be replaced again by the next op.
      local map = op.map or {}
      local swaps = {}
      local count = 0
      for fromHex, toHex in pairs(map) do
        swaps[color_value(spr, fromHex)] = color_value(spr, toHex)
        count = count + 1
      end
      if count == 0 then
        fail("invalid_args", "remap needs a `map` of from-colour to to-colour, for example {\"#ff0000\": \"#00ff00\"}.",
          { opIndex = i })
      end
      local region = region_of(op)
      for py = 0, img.height - 1 do
        for px = 0, img.width - 1 do
          local to = swaps[img:getPixel(px, py)]
          if to ~= nil and in_region(region, px, py) and inside(px, py) then
            write(img, px, py, to)
          end
        end
      end

    elseif kind == "frame" then
      -- The bevelled box seven of the app's sprites share: notched corners, an
      -- outline, a lit top-left face and a shaded bottom-right one.
      local fx, fy = tonumber(op.x) or 0, tonumber(op.y) or 0
      local fw, fh = tonumber(op.width) or 0, tonumber(op.height) or 0
      if fw < 3 or fh < 3 then
        fail("invalid_args", "frame needs a width and height of at least 3.",
          { opIndex = i, width = fw, height = fh })
      end
      local cut = math.floor(tonumber(op.cut) or 1)
      cut = math.max(0, math.min(cut, math.floor(math.min(fw, fh) / 2)))
      local openBottom = op.openBottom == true
      local outline = color_value(spr, op.outline or "#000000")
      local hi = op.hi and color_value(spr, op.hi) or nil
      local lo = op.lo and color_value(spr, op.lo) or nil
      local body = op.body and color_value(spr, op.body) or nil

      local function shape(px, py)
        if px < 0 or py < 0 or px >= fw or py >= fh then return false end
        if cut > 0 then
          if px + py < cut then return false end
          if (fw - 1 - px) + py < cut then return false end
          if px + (fh - 1 - py) < cut then return false end
          if (fw - 1 - px) + (fh - 1 - py) < cut then return false end
        end
        return true
      end

      -- openBottom drops the last row from what is painted, but not from the
      -- shape: dropping it from both would just move the bottom edge one row up.
      local function paintable(px, py)
        if not shape(px, py) then return false end
        if openBottom and py == fh - 1 then return false end
        return true
      end

      local function edge_at(px, py)
        if not shape(px, py) then return false end
        return not shape(px - 1, py) or not shape(px + 1, py)
          or not shape(px, py - 1) or not shape(px, py + 1)
      end

      for py = 0, fh - 1 do
        for px = 0, fw - 1 do
          if paintable(px, py) then
            local colour
            if edge_at(px, py) then
              colour = outline
            elseif hi and (edge_at(px - 1, py) or edge_at(px, py - 1)) then
              colour = hi
            elseif lo and (edge_at(px + 1, py) or edge_at(px, py + 1)) then
              colour = lo
            else
              colour = body
            end
            if colour ~= nil then write(img, fx + px, fy + py, colour) end
          end
        end
      end

    elseif kind == "outline" then
      -- Grow the silhouette by one pixel, or recolour its edge. The source is
      -- snapshotted first, or the outline eats its own edge.
      local colour = color_value(spr, op.color or "#000000")
      local side = tostring(op.side or "outside")
      local diagonals = op.diagonals == true

      local src = Image(img.width, img.height, spr.colorMode)
      src:drawImage(img, Point(0, 0))

      local function solid(x, y)
        if x < 0 or y < 0 or x >= src.width or y >= src.height then return false end
        return app.pixelColor.rgbaA(src:getPixel(x, y)) > 0
      end

      local offsets = { { 1, 0 }, { -1, 0 }, { 0, 1 }, { 0, -1 } }
      if diagonals then
        offsets[#offsets + 1] = { 1, 1 }
        offsets[#offsets + 1] = { -1, 1 }
        offsets[#offsets + 1] = { 1, -1 }
        offsets[#offsets + 1] = { -1, -1 }
      end

      for y = 0, src.height - 1 do
        for x = 0, src.width - 1 do
          local here = solid(x, y)
          local edge = false
          for _, o in ipairs(offsets) do
            if solid(x + o[1], y + o[2]) ~= here then edge = true; break end
          end
          if edge then
            if side == "inside" then
              if here then write(img, x, y, colour) end
            else
              if not here then write(img, x, y, colour) end
            end
          end
        end
      end

    elseif kind == "shade" then
      -- Lighten or darken with a hue shift: the same hue darker reads as a
      -- brightness slider.
      local amount = tonumber(op.amount) or 0
      if amount == 0 then
        fail("invalid_args", "shade needs a non-zero amount, for example -0.2 for a shadow or 0.2 for a highlight.",
          { opIndex = i })
      end
      if spr.colorMode == ColorMode.INDEXED then
        fail("unsupported", "shade needs an RGB document: an indexed palette cannot hold the shifted colour.",
          { opIndex = i, hint = "convert the sprite to RGB, or move the ramp into the palette first" })
      end

      local cache = {}
      local function shifted(pixel)
        if cache[pixel] ~= nil then return cache[pixel] end
        local r = app.pixelColor.rgbaR(pixel)
        local g = app.pixelColor.rgbaG(pixel)
        local b = app.pixelColor.rgbaB(pixel)
        local a = app.pixelColor.rgbaA(pixel)
        local nr, ng, nb
        if amount < 0 then
          local k = math.min(1, -amount)
          -- Shadows fall on red first and blue last, so the shadow cools.
          nr = r * (1 - k * 0.62)
          ng = g * (1 - k * 0.50)
          nb = b * (1 - k * 0.30)
        else
          local k = math.min(1, amount)
          -- Highlights rise on red and green first, so the light warms.
          nr = r + (255 - r) * k * 0.55
          ng = g + (255 - g) * k * 0.50
          nb = b + (255 - b) * k * 0.35
        end
        local function byte(v)
          v = math.floor(v + 0.5)
          if v < 0 then return 0 end
          if v > 255 then return 255 end
          return v
        end
        local out = app.pixelColor.rgba(byte(nr), byte(ng), byte(nb), a)
        cache[pixel] = out
        return out
      end

      local region = region_of(op)
      for py = 0, img.height - 1 do
        for px = 0, img.width - 1 do
          local p = img:getPixel(px, py)
          if app.pixelColor.rgbaA(p) > 0 and in_region(region, px, py) and inside(px, py) then
            write(img, px, py, shifted(p))
          end
        end
      end

    elseif kind == "blit" then
      -- Copy a rectangle of another cel. `source` is an object, so it cannot be
      -- confused with replace's from/to strings.
      local source = op.source or {}
      local srcLayer = require_layer(spr, source.layer)
      local srcFrame = frame_index(spr, { frame = source.frame })
      local srcCel = srcLayer:cel(spr.frames[srcFrame])
      if not srcCel then
        fail("invalid_args", "blit source '" .. tostring(source.layer) .. "' has no cel on frame " .. srcFrame,
          { opIndex = i })
      end

      -- Snapshot the source, so blitting a layer onto itself reads what was
      -- there.
      local src = Image(srcCel.image.width, srcCel.image.height, spr.colorMode)
      src:drawImage(srcCel.image, Point(0, 0))

      local sx = tonumber(source.x) or 0
      local sy = tonumber(source.y) or 0
      local w = tonumber(source.width) or src.width
      local h = tonumber(source.height) or src.height
      local dx = tonumber(op.x) or 0
      local dy = tonumber(op.y) or 0

      for by = 0, h - 1 do
        for bx = 0, w - 1 do
          local px, py = sx + bx - srcCel.position.x, sy + by - srcCel.position.y
          if px >= 0 and py >= 0 and px < src.width and py < src.height then
            local p = src:getPixel(px, py)
            if app.pixelColor.rgbaA(p) > 0 then
              write(img, dx + bx, dy + by, p)
            end
          end
        end
      end

    elseif kind == "shift" then
      -- Copy the cel onto itself, moved. With onlyTransparent it paints behind
      -- the art, which is a drop shadow in one call instead of a second layer.
      local dx = math.floor(tonumber(op.dx) or 0)
      local dy = math.floor(tonumber(op.dy) or 0)
      if dx == 0 and dy == 0 then
        fail("invalid_args", "shift needs a non-zero dx or dy.", { opIndex = i })
      end
      local onlyTransparent = op.onlyTransparent ~= false
      local colour = nil
      if op.color ~= nil then colour = color_value(spr, op.color) end

      local src = Image(img.width, img.height, spr.colorMode)
      src:drawImage(img, Point(0, 0))

      for py = 0, src.height - 1 do
        for px = 0, src.width - 1 do
          local p = src:getPixel(px, py)
          if app.pixelColor.rgbaA(p) > 0 then
            local tx, ty = px + dx, py + dy
            local blocked = false
            if onlyTransparent and tx >= 0 and ty >= 0 and tx < img.width and ty < img.height then
              blocked = app.pixelColor.rgbaA(img:getPixel(tx, ty)) > 0
            end
            if not blocked then
              write(img, tx, ty, colour or p)
            end
          end
        end
      end

    elseif kind == "fill" then
      -- Flood fill from one point, four-connected: otherwise it leaks through a
      -- diagonal gap and floods the sprite.
      local sx, sy = tonumber(op.x) or 0, tonumber(op.y) or 0
      local colour = color_value(spr, op.color)
      local tolerance = tonumber(op.tolerance) or 0
      if sx < 0 or sy < 0 or sx >= img.width or sy >= img.height then
        fail("invalid_args", "fill starts at (" .. sx .. "," .. sy .. "), which is outside the cel.", { opIndex = i })
      end

      local seed = img:getPixel(sx, sy)
      local function near(p)
        if p == seed then return true end
        if tolerance <= 0 then return false end
        local dr = app.pixelColor.rgbaR(p) - app.pixelColor.rgbaR(seed)
        local dg = app.pixelColor.rgbaG(p) - app.pixelColor.rgbaG(seed)
        local db = app.pixelColor.rgbaB(p) - app.pixelColor.rgbaB(seed)
        local da = app.pixelColor.rgbaA(p) - app.pixelColor.rgbaA(seed)
        return math.sqrt(dr * dr + dg * dg + db * db + da * da) <= tolerance
      end

      local seen = {}
      local stack = { { sx, sy } }
      while #stack > 0 do
        local point = table.remove(stack)
        local x, y = point[1], point[2]
        local key = y * img.width + x
        if not seen[key] and x >= 0 and y >= 0 and x < img.width and y < img.height then
          seen[key] = true
          if near(img:getPixel(x, y)) then
            write(img, x, y, colour)
            stack[#stack + 1] = { x + 1, y }
            stack[#stack + 1] = { x - 1, y }
            stack[#stack + 1] = { x, y + 1 }
            stack[#stack + 1] = { x, y - 1 }
          end
        end
      end

    elseif kind == "polyline" then
      -- Many line segments in one op, so a contour needs no op per corner.
      local colour = color_value(spr, op.color)
      local points = op.points or {}
      local prev = nil
      for _, point in ipairs(points) do
        local x, y = tonumber(point[1]), tonumber(point[2])
        if x and y then
          if prev then
            local x0, y0 = prev[1], prev[2]
            local dx, dy = math.abs(x - x0), math.abs(y - y0)
            local stepX = x0 < x and 1 or -1
            local stepY = y0 < y and 1 or -1
            local err = dx - dy
            local cx, cy = x0, y0
            while true do
              write(img, cx, cy, colour)
              if cx == x and cy == y then break end
              local e2 = 2 * err
              if e2 > -dy then err = err - dy; cx = cx + stepX end
              if e2 < dx then err = err + dx; cy = cy + stepY end
            end
          else
            write(img, x, y, colour)
          end
          prev = { x, y }
        end
      end

    elseif kind == "dither" then
      -- A transition between two ramp steps. Only for large flat areas; anywhere
      -- else it is noise.
      local x0, y0 = tonumber(op.x) or 0, tonumber(op.y) or 0
      local w, h = tonumber(op.width) or 0, tonumber(op.height) or 0
      local a = color_value(spr, op.from)
      local b = color_value(spr, op.to)
      local pattern = tostring(op.pattern or "checker")

      local bayer = {
        { 0, 8, 2, 10 },
        { 12, 4, 14, 6 },
        { 3, 11, 1, 9 },
        { 15, 7, 13, 5 },
      }

      for y = y0, y0 + h - 1 do
        for x = x0, x0 + w - 1 do
          local takeB
          if pattern == "bayer4" then
            takeB = bayer[((y - y0) % 4) + 1][((x - x0) % 4) + 1] < 8
          else
            takeB = ((x + y) % 2) == 0
          end
          write(img, x, y, takeB and b or a)
        end
      end

    elseif kind == "gradient" then
      -- A stepped gradient between two colours: a smooth ramp is dozens of
      -- colours nobody chose.
      local x0, y0 = tonumber(op.x) or 0, tonumber(op.y) or 0
      local w, h = tonumber(op.width) or 0, tonumber(op.height) or 0
      local a = color_value(spr, op.from)
      local b = color_value(spr, op.to)
      local vertical = tostring(op.direction or "vertical") == "vertical"
      local span = vertical and h or w
      if span < 1 then fail("invalid_args", "gradient needs a width or height.", { opIndex = i }) end
      local steps = math.max(2, math.min(span, math.floor(tonumber(op.steps) or span)))

      local function lerp_channel(ca, cb, t)
        local v = ca + (cb - ca) * t
        v = math.floor(v + 0.5)
        if v < 0 then return 0 end
        if v > 255 then return 255 end
        return v
      end

      for y = y0, y0 + h - 1 do
        for x = x0, x0 + w - 1 do
          local i2 = vertical and (y - y0) or (x - x0)
          local band = math.floor(i2 * steps / span)
          local t = steps == 1 and 0 or band / (steps - 1)
          local colour = app.pixelColor.rgba(
            lerp_channel(app.pixelColor.rgbaR(a), app.pixelColor.rgbaR(b), t),
            lerp_channel(app.pixelColor.rgbaG(a), app.pixelColor.rgbaG(b), t),
            lerp_channel(app.pixelColor.rgbaB(a), app.pixelColor.rgbaB(b), t),
            255)
          write(img, x, y, colour)
        end
      end

    elseif kind == "hue_shift" then
      -- Rotate hue without touching value, for recolouring a team colour.
      local degrees = tonumber(op.degrees) or 0
      if spr.colorMode == ColorMode.INDEXED then
        fail("unsupported", "hue_shift needs an RGB document.", { opIndex = i })
      end
      local cache = {}
      local region = region_of(op)
      for py = 0, img.height - 1 do
        for px = 0, img.width - 1 do
          local p = img:getPixel(px, py)
          if app.pixelColor.rgbaA(p) > 0 and in_region(region, px, py) and inside(px, py) then
            local out = cache[p]
            if out == nil then
              local r, g, b = app.pixelColor.rgbaR(p), app.pixelColor.rgbaG(p), app.pixelColor.rgbaB(p)
              local mx, mn = math.max(r, g, b), math.min(r, g, b)
              local v = mx / 255
              local s = mx == 0 and 0 or (mx - mn) / mx
              local hdg
              if mx == mn then
                hdg = 0
              elseif mx == r then
                hdg = (60 * ((g - b) / (mx - mn))) % 360
              elseif mx == g then
                hdg = 60 * ((b - r) / (mx - mn)) + 120
              else
                hdg = 60 * ((r - g) / (mx - mn)) + 240
              end
              hdg = (hdg + degrees) % 360

              local c = v * s
              local xx = c * (1 - math.abs(((hdg / 60) % 2) - 1))
              local m = v - c
              local rr, gg, bb
              if hdg < 60 then rr, gg, bb = c, xx, 0
              elseif hdg < 120 then rr, gg, bb = xx, c, 0
              elseif hdg < 180 then rr, gg, bb = 0, c, xx
              elseif hdg < 240 then rr, gg, bb = 0, xx, c
              elseif hdg < 300 then rr, gg, bb = xx, 0, c
              else rr, gg, bb = c, 0, xx end
              out = app.pixelColor.rgba(
                math.floor((rr + m) * 255 + 0.5),
                math.floor((gg + m) * 255 + 0.5),
                math.floor((bb + m) * 255 + 0.5),
                app.pixelColor.rgbaA(p))
              cache[p] = out
            end
            write(img, px, py, out)
          end
        end
      end

    elseif kind == "desaturate" then
      -- Pull colour toward its luminance by amount (0 keeps, 1 is grey), which is
      -- how you audition value contrast.
      local amount = math.max(0, math.min(1, tonumber(op.amount) or 0.5))
      if spr.colorMode == ColorMode.INDEXED then
        fail("unsupported", "desaturate needs an RGB document.", { opIndex = i })
      end
      local cache = {}
      local region = region_of(op)
      for py = 0, img.height - 1 do
        for px = 0, img.width - 1 do
          local p = img:getPixel(px, py)
          if app.pixelColor.rgbaA(p) > 0 and in_region(region, px, py) and inside(px, py) then
            local out = cache[p]
            if out == nil then
              local r, g, b = app.pixelColor.rgbaR(p), app.pixelColor.rgbaG(p), app.pixelColor.rgbaB(p)
              local lum = 0.2126 * r + 0.7152 * g + 0.0722 * b
              local function mix(c)
                local v = math.floor(c + (lum - c) * amount + 0.5)
                if v < 0 then return 0 end
                if v > 255 then return 255 end
                return v
              end
              out = app.pixelColor.rgba(mix(r), mix(g), mix(b), app.pixelColor.rgbaA(p))
              cache[p] = out
            end
            write(img, px, py, out)
          end
        end
      end

    else
      fail("invalid_args", "Unknown draw kind '" .. tostring(kind) .. "'.", { opIndex = i })
    end
  end

  return { pixelsChanged = changed, pixelsWritten = written, opsApplied = #ops }
end

-- Palette ---------------------------------------------------------------

H["palette.get"] = function(args)
  return { colors = palette_hexes(asprite(args)) }
end

H["palette.set"] = function(args)
  local spr = asprite(args)
  local colours = args.colors or {}
  if #colours == 0 then fail("invalid_args", "palette.set needs at least one colour.") end

  local pal = Palette(#colours)
  for i, hex in ipairs(colours) do
    local r, g, b, a = hex_components(hex)
    pal:setColor(i - 1, Color(r, g, b, a))
  end
  spr:setPalette(pal)
  return { colors = palette_hexes(spr) }
end

-- palette.snap moves every pixel onto the nearest palette entry.
--
-- This is what turns a stray anti-aliased edge into clean pixel art: after a
-- resize or a paint-over, the art and the palette have drifted, and the fix is
-- to pull the art back rather than invent new palette entries for it.
H["palette.snap"] = function(args)
  local spr = asprite(args)
  local pal = spr.palettes[1]
  if not pal or #pal == 0 then
    fail("invalid_args", "The document has no palette to snap to.")
  end

  local entries = {}
  for i = 0, #pal - 1 do
    local c = pal:getColor(i)
    entries[#entries + 1] = {
      index = i,
      r = c.red, g = c.green, b = c.blue,
      hex = string.format("#%02x%02x%02x", c.red, c.green, c.blue),
    }
  end

  local onlyLayer = args.layer
  local onlyFrame = tonumber(args.frame)

  local cache = {}
  local function snap(pixel)
    local cached = cache[pixel]
    if cached ~= nil then return cached end
    local r = app.pixelColor.rgbaR(pixel)
    local g = app.pixelColor.rgbaG(pixel)
    local b = app.pixelColor.rgbaB(pixel)
    local best, bestDistance = entries[1], math.huge
    for _, e in ipairs(entries) do
      local dr, dg, db = e.r - r, e.g - g, e.b - b
      local d = dr * dr + dg * dg + db * db
      if d < bestDistance then best, bestDistance = e, d end
    end
    local out
    if spr.colorMode == ColorMode.INDEXED then
      out = app.pixelColor.index(best.index)
    else
      out = app.pixelColor.rgba(best.r, best.g, best.b, app.pixelColor.rgbaA(pixel))
    end
    cache[pixel] = out
    return out
  end

  local changed, visited = 0, 0
  local moves = {}

  local function snap_image(img)
    for y = 0, img.height - 1 do
      for x = 0, img.width - 1 do
        local p = img:getPixel(x, y)
        if app.pixelColor.rgbaA(p) > 0 then
          visited = visited + 1
          local out = snap(p)
          if out ~= p then
            img:putPixel(x, y, out)
            changed = changed + 1
            local from = color_hex(spr, p) or "#000000"
            local to = color_hex(spr, out) or "#000000"
            local key = from .. ">" .. to
            moves[key] = (moves[key] or 0) + 1
          end
        end
      end
    end
  end

  local function walk(layers)
    for _, layer in ipairs(layers) do
      if layer.isGroup then
        walk(layer.layers)
      elseif onlyLayer == nil or onlyLayer == "" or layer.name == onlyLayer then
        for _, cel in ipairs(layer.cels) do
          if onlyFrame == nil or cel.frameNumber == onlyFrame then
            snap_image(cel.image)
          end
        end
      end
    end
  end
  walk(spr.layers)

  local distinct = 0
  for _ in pairs(moves) do distinct = distinct + 1 end
  return {
    pixelsChanged = changed,
    pixelsVisited = visited,
    snappedColours = distinct,
    moves = moves,
  }
end

--------------------------------------------------------------------------------
-- Dispatch
--------------------------------------------------------------------------------

local function handle_command(frame)
  local handler = H[frame.cmd]
  if not handler then
    return {
      id = frame.id,
      ok = false,
      error = {
        code = "unsupported_command",
        message = "This SpriteWare extension does not implement '" .. tostring(frame.cmd) .. "'.",
        details = { command = frame.cmd, extensionVersion = EXTENSION_VERSION },
      },
    }
  end

  local ok, result = pcall(handler, frame.args or {})
  if not ok then
    local e = result
    if type(e) == "table" and e.__spriteware then
      return {
        id = frame.id, ok = false,
        error = { code = e.code, message = e.message, details = e.details or {} },
      }
    end
    return {
      id = frame.id, ok = false,
      error = {
        code = "lua_error",
        message = tostring(result),
        details = { command = frame.cmd, extensionVersion = EXTENSION_VERSION },
      },
    }
  end

  local payload = result or {}
  -- Arrays (doc.list, layer.list) have no named document to report, and a field
  -- on an array breaks its decode on the far side.
  if type(payload) == "table" and not is_array(payload) then
    -- The document the command acted on, not whichever happened to be active.
    -- Reading `current` reported the active document, so `draw` aimed at one
    -- file with `sprite` and still said the other file was the target — the one
    -- field meant to answer "did it touch the right document" was wrong.
    local target
    local wanted = frame.args and frame.args.sprite
    if wanted ~= nil and tostring(wanted) ~= "" then
      local ok, found = pcall(match_sprite, tostring(wanted))
      if ok then target = found end
    end
    if target == nil then target = current end
    if target == nil then
      local ok, active = pcall(function() return app.activeSprite end)
      if ok then target = active end
    end
    payload.target = target_ref(target)
  end
  return { id = frame.id, ok = true, data = payload }
end

SPRITEWARE.protocol = PROTOCOL_VERSION
SPRITEWARE.version = EXTENSION_VERSION
SPRITEWARE.features = FEATURES
SPRITEWARE.handlers = H
SPRITEWARE.handleCommand = handle_command
SPRITEWARE.encodeJson = encode_json
SPRITEWARE.toPlain = to_plain
SPRITEWARE.array = arr

--------------------------------------------------------------------------------
-- Live transport
--
-- Aseprite has no LuaSocket: `require("socket")` fails in every build, so the
-- first version of this file connected to nothing at all. What Aseprite does
-- ship is a WebSocket class (IXWebSocket), so live mode rides on that.
--
-- Skipped in batch mode, where app.isUIAvailable is false and headless/runner.lua
-- owns stdin/stdout.
--------------------------------------------------------------------------------

local BRIDGE_PORT = tonumber(app.params["bridgePort"] or "") or 9934 -- spriteware:bridge-port

local function start_live()
  if not app.isUIAvailable then return end

  if type(WebSocket) ~= "function" then
    -- Say it once, in the console. A silent return is what made this look like
    -- "the agent just doesn't draw anything".
    print("SpriteWare: this Aseprite build has no WebSocket support, so live mode cannot connect. Headless mode still works.")
    return
  end

  local socket = nil
  local open = false
  local connecting = false
  SPRITEWARE.live = SPRITEWARE.live or {}

  local function send(payload)
    if not open or not socket then return end
    pcall(function() socket:sendText(payload) end)
  end

  local function handle_message(data)
    local decodedOk, decoded = pcall(json.decode, data)
    if not decodedOk or not decoded then return end
    local frame = to_plain(decoded)
    if type(frame) ~= "table" or frame.id == nil then return end
    local answered, reply = pcall(handle_command, frame)
    if not answered then
      reply = {
        id = frame.id, ok = false,
        error = { code = "lua_error", message = tostring(reply), details = {} },
      }
    end
    -- A script edit changes the image, not Aseprite's idea of when to paint:
    -- without this the window keeps showing the previous canvas until the user
    -- moves it.
    pcall(function() app.refresh() end)
    send(encode_json(reply))
  end

  local function drop()
    if socket then
      pcall(function() socket:close() end)
    end
    socket = nil
    SPRITEWARE.live.socket = nil
    open = false
    connecting = false
  end

  -- A dropped connection ends IXWebSocket's own thread for good: its reconnect
  -- loop gives up after a close, and reusing the object does nothing because
  -- start() refuses to spawn a second thread. A new object is the only way back,
  -- so the watchdog below builds one whenever the socket is not open.
  local function attempt()
    drop()
    connecting = true
    local made, err = pcall(function()
      socket = WebSocket {
        url = "ws://127.0.0.1:" .. BRIDGE_PORT .. "/extension",
        minreconnectwait = 0.2,
        maxreconnectwait = 2,
        onreceive = function(kind, data)
          if kind == WebSocketMessageType.OPEN then
            open = true
            connecting = false
            send(encode_json({
              type = "hello",
              protocol = PROTOCOL_VERSION,
              extensionVersion = EXTENSION_VERSION,
              asepriteVersion = tostring(app.version),
              headless = false,
              features = FEATURES,
            }))
          elseif kind == WebSocketMessageType.TEXT then
            handle_message(data)
          elseif kind == WebSocketMessageType.CLOSE or kind == WebSocketMessageType.ERROR then
            open = false
            connecting = false
          end
        end,
      }
    end)
    if not made or not socket then
      connecting = false
      print("SpriteWare: could not create the live socket: " .. tostring(err))
      return
    end
    -- Anchored on the module table on purpose: a local would be collected the
    -- moment start_live returns, and __gc stops both the socket and the timer.
    SPRITEWARE.live.socket = socket
    pcall(function() socket:connect() end)
  end

  SPRITEWARE.live = SPRITEWARE.live or {}
  local watchdog = Timer {
    interval = 1.0,
    -- Aseprite reads this field as lowercase `ontick` (timer_class.cpp); an
    -- `onTick` key connects nothing and the timer ticks into the void.
    ontick = function()
      -- A killed app leaves this end of the socket looking open, and IXWebSocket
      -- only raises Close when its own transport notices. A ping it cannot send
      -- is the earliest reliable sign.
      if open and socket then
        local pinged = pcall(function() socket:sendPing("spriteware") end)
        if not pinged then open = false end
      end

      if not open and not connecting then attempt() end
    end,
  }
  SPRITEWARE.live.watchdog = watchdog
  watchdog:start()
  attempt()
end

-- Wrapped: a failed transport start must not stop the handler table existing,
-- or every tool goes dead over a network problem.
pcall(start_live)
