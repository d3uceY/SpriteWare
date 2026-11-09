---
name: SpriteWare
description: A quiet pixel-art control panel for the MCP server that lets an AI assistant draw in Aseprite.
colors:
  window-beige: "#d3cbbe"
  window-hot: "#faf0e6"
  workspace-slate: "#c1cbd0"
  slate-highlight: "#cedae2"
  key-grey: "#c6c6c6"
  key-grey-lo: "#8e8e8e"
  well-white: "#ffffff"
  outline-black: "#000000"
  ink: "#101417"
  slate-ink: "#334149"
  lamp-red: "#ff5555"
  lamp-red-deep: "#c23838"
  link-blue: "#2c4c91"
  link-blue-deep: "#213a70"
  hover-cream: "#ffebb6"
  lamp-green: "#64c864"
  text-muted: "#4f473b"
  text-ok: "#1a5a1a"
  text-error: "#a11d1d"
typography:
  display:
    fontFamily: "Pixelify Sans Variable, ui-monospace, monospace"
    fontSize: "20px"
    fontWeight: 600
    lineHeight: 1.2
  title:
    fontFamily: "Pixelify Sans Variable, ui-monospace, monospace"
    fontSize: "clamp(40px, 5.6vw, 58px)"
    fontWeight: 700
    lineHeight: 0.92
    letterSpacing: "0.015em"
  body:
    fontFamily: "Albert Sans Variable, ui-sans-serif, system-ui, sans-serif"
    fontSize: "14px"
    fontWeight: 500
    lineHeight: 1.5
  label:
    fontFamily: "Pixelify Sans Variable, ui-monospace, monospace"
    fontSize: "13px"
    fontWeight: 600
    letterSpacing: "0.1em"
  data:
    fontFamily: "IBM Plex Mono, ui-monospace, Consolas, monospace"
    fontSize: "13px"
    fontWeight: 500
    lineHeight: 1.65
rounded:
  none: "0px"
spacing:
  xs: "4px"
  sm: "8px"
  md: "12px"
  lg: "16px"
  xl: "24px"
  xxl: "32px"
components:
  button-primary:
    backgroundColor: "{colors.lamp-red}"
    textColor: "{colors.outline-black}"
    typography: "{typography.display}"
    rounded: "{rounded.none}"
    height: "42px"
    padding: "0 12px"
  button-primary-hover:
    backgroundColor: "{colors.lamp-red}"
  button-secondary:
    backgroundColor: "{colors.key-grey}"
    textColor: "{colors.outline-black}"
    typography: "{typography.display}"
    rounded: "{rounded.none}"
    height: "42px"
    padding: "0 12px"
  button-link:
    backgroundColor: "transparent"
    textColor: "{colors.link-blue}"
    typography: "{typography.display}"
    height: "28px"
    padding: "0 6px"
  button-link-hover:
    backgroundColor: "{colors.hover-cream}"
  card:
    backgroundColor: "{colors.window-hot}"
    textColor: "{colors.outline-black}"
    rounded: "{rounded.none}"
    padding: "4px 8px 8px"
  well:
    backgroundColor: "{colors.well-white}"
    textColor: "{colors.outline-black}"
    typography: "{typography.data}"
    rounded: "{rounded.none}"
  tab:
    backgroundColor: "{colors.key-grey}"
    textColor: "{colors.outline-black}"
    typography: "{typography.display}"
    height: "40px"
  tab-active:
    backgroundColor: "{colors.workspace-slate}"
    textColor: "{colors.outline-black}"
    height: "40px"
---

# Design System: SpriteWare

## Overview

**Creative North Star: "The Tool Window"**

SpriteWare is a control panel for Aseprite's own MCP server, so it dresses as Aseprite: a beige window, a slate workspace, grey keys with black outlines, and a handful of drawn sprites. It borrows the host program's light theme on purpose. A person who opens it next to Aseprite should feel it belongs to the same toolbox, and should never have to learn a second visual language to read a status or press a button.

The product is Operate-mode: a person starts a server, copies a config, and leaves. The interface therefore does one thing per screen, names it in a display title, and gets out of the way. Pixel art carries the personality (the mascot, the wordmark, the frames, the icons) and every other surface is flat, matte and legible. Nothing glows, shimmers or decorates. An earlier dark, starfield version was rejected because the effects cost readability.

**Key Characteristics:**
- Aseprite's light theme, translated: beige window, slate workspace, white wells.
- Pixel art at one scale: every frame is two screen pixels per art pixel, so edges stay crisp.
- Square, bevelled, outlined. Corners are stepped notches drawn in the art, never radii.
- One red key per screen. Everything else is grey, beige or white.
- Motion is stepped and short: sprites animate, surfaces do not.

## Colors

The palette is Aseprite's light theme (`aseprite-theme-light.xml`) with two additions: dark text tones for hints, and a lamp green for "live".

### Primary
- **Lamp Red** (#ff5555): the single action key on a screen, the selected list row, the "needs attention" lamp, text selection. Aseprite's own selection colour, so it already means "this one".

### Neutral
- **Window Beige** (#d3cbbe): the window, the tab strip and every card. Text on it is black.
- **Window Hot** (#faf0e6): card highlight edge, dialog fill, the mascot's body.
- **Workspace Slate** (#c1cbd0): the stage behind the cards, and the open tab, so the tab reads as the page it opens.
- **Key Grey** (#c6c6c6): secondary keys, closed tabs, the open segment of a segmented control.
- **Key Grey Low** (#8e8e8e): the bottom-right bevel of every grey key.
- **Well White** (#ffffff): anything you read or type: addresses, paths, snippets, the tool list, menus.
- **Outline Black** (#000000): every outline and every label on a key.
- **Ink** (#101417): text set directly on the slate (page title).
- **Slate Ink** (#334149): the page lead and other secondary text on the slate, one step softer than Ink without losing contrast.
- **Muted Brown** (#4f473b): hints on beige. 5.4:1 on Window Beige.

### Accents
- **Link Blue** (#2c4c91): copy links and small section labels. Aseprite's separator-label colour.
- **Hover Cream** (#ffebb6): hover fill for rows, links and menu items. Aseprite's hot-item colour.
- **Lamp Green** (#64c864): the server is listening; the address well tints green (#e6f7e3).

### Named Rules
**The One Red Key Rule.** Lamp Red appears on one filled key per screen, plus the selected row in an open list. If a second red key is wanted, one of them is not the primary action.

**The Ink On Slate Rule.** Text set on the slate is Ink or Slate Ink, never white and never plum. White on the slate is 3.3:1 and fails; the theme's own plum is worse.

## Typography

**Display Font:** Pixelify Sans (variable) for titles inside cards, keys, tabs and labels.
**Body Font:** Albert Sans (variable) for sentences.
**Data Font:** IBM Plex Mono for paths, addresses, config text and tool names.
**Title Face:** Pixelify Sans at display scale (40-58px, 700, uppercase). The 41-cell drawn glyph sheet (`glyphs.png`, A to Z, 0 to 9, space . : / -) is now only the wordmark: at page-title size its pale shadow muddied the letters and its 4px cells could not hold an edge across window widths.

**Character:** a pixel face for anything you press or navigate, a humanist sans for anything you read, a mono for anything you copy. Three voices, each tied to one job.

### Hierarchy
- **Title** (Pixelify 700, 40-58px, 0.92 line-height): the page name, uppercase. One per screen.
- **Display** (Pixelify 700, 22px, 1.15): card titles and the status word.
- **Key / Tab** (Pixelify 600, 15 to 17px): labels on keys and tabs.
- **Body** (Albert Sans 500, 14px, 1.5): sentences. Lead line is 15px, capped at 56ch.
- **Label** (Pixelify 600, 13px, 0.1em, uppercase, Link Blue): the small heading over a field.
- **Data** (Plex Mono 500, 12.5 to 14px, 1.65 in blocks): everything copyable.

### Named Rules
**The Display Title Rule.** The page title is set in Pixelify Sans, never from the glyph sheet. Body copy, labels, keys and the wordmark keep their own voices so the hierarchy never depends on a single asset.

## Layout

One window, one column. A beige chrome strip holds the mark and the connection state, a tab strip below it names the three screens, and the slate stage under that holds a single column of cards, 720px at most, centred.

Spacing is a short scale: 4, 8, 12, 16, 24, 32. Cards stack at 16. Inside a card, groups sit 12 apart. The page head is the display title, a line of intent, and one hairline rule under both; there are no breadcrumbs or numerals.

Tabs are a real tablist. The open tab is slate, 6px taller, and covers the 2px black rail under the strip so it joins the stage; its label is Link Blue, its icon at full colour, and its four-frame sprite plays. Closed tabs keep muted labels and dim their icons to 62%. At 760px and below the page loses side padding and the title steps down to 44px at most; at 560px the wordmark gives way to the mascot alone.

### Named Rules
**The One Column Rule.** There is no sidebar, grid of cards or second column. If a screen needs more, it is a longer column.

## Elevation & Depth

Depth is drawn, not blurred. Every key and card has a bevel baked into its frame (light top-left, dark bottom-right) and a hard 2px black lip under it. Wells are the inverse: dark top-left, so they read as recessed.

### Shadow Vocabulary
- **Lip** (`filter: drop-shadow(0 2px 0 #000)`): under keys and cards. A drop-shadow, not a box-shadow, so it follows the notched corners instead of squaring them.
- **Press** (`transform: translateY(2px)` with the lip removed): a key goes down by exactly its lip.
- **Dialog Lip** (`drop-shadow(0 4px 0 #000)`): the dialog sits one step higher than a card.

### Named Rules
**The Hard Lip Rule.** No blur, no spread, no translucent shadow. A shadow is a solid black row or it is not there.

## Shapes

Nothing is rounded. Every corner is a stepped notch drawn into the sprite: two pixels cut on keys, wells and cards, three on the dialog. Frames are PNGs sliced with `border-image` at two screen pixels per art pixel, so one asset draws a box of any size with its corners intact. Dots are squares. The only circles are the lamps, which are drawn.

## Components

### Buttons
- **Primary** (`.btn`): the red 12x12 key, 42px high, black Pixelify label. Hover brightens 8%; press moves it down 2px.
- **Secondary** (`.btn--ghost`): the grey key, same size and behaviour.
- **Link** (`.btn--link`): Link Blue Pixelify, no frame. Hover fills Hover Cream and underlines.
- **Icon keys** carry a 16px drawn icon (play, stop, search, folder) at one screen pixel per art pixel.

### Cards
Beige window frame, black outline, Window Hot highlight edge, 2px lip. A card has an optional Display title and one line beneath it. No nested cards.

### Wells
White, black outline, recessed bevel. Used for the address (green-tinted while listening), paths, config snippets, the path field, port fields, segmented controls, menus, the tool list, and the state plate in the chrome. Error notes use a pink well with a red edge.

### Navigation
Four tabs: Server, Aseprite, Tools, Settings. Closed tabs are grey chips on the beige strip with muted labels and dimmed icons; the open tab is slate with a Link Blue label and a lit four-frame icon (the nav sprite plays while open or hovered). While Aseprite is not yet found, Server and Tools are disabled; Aseprite and Settings stay open, because a port that is taken is a reason setup cannot finish.

### Fields
A labelled control: a Display label in Link Blue above a well. The host half of a port is fixed, so it is printed inside the well (`.numfield__prefix`) and only the number is typed. Focus rings the well, the same outline a path field uses.

### Toggle
A drawn switch inside a grey key, with the sentence describing the current state beside it. One asset, five frames — off, three sliding poses, on — so the knob cannot show a state the app is not in. The sheet's background position steps through the poses on toggle, and settles on frame 1 or frame 5. It carries `role="switch"` and `aria-checked`.

### Assistant Picker
A grey key that opens a white list. Hover is Hover Cream; the chosen row is Lamp Red. The list flips upward when there is no room below.

### Dialog
A double-edged cream window (black, white, grey, black), over a 45% black scrim. It names a path the app guessed and asks for confirmation. Focus is trapped inside; Escape declines.

### Signature: The Mascot
A cream blob with an antenna, 16x16, three frames: eyes open, blinking, looking aside. It sits in the chrome and in the dialog. It blinks on a 5.2s loop, stepped.

### Signature: Lamps
Eight-pixel gems: green and pulsing while the server listens, grey when stopped, blinking red when Aseprite is missing.

## Do's and Don'ts

### Do:
- **Do** keep text on the slate Ink (#101417) and text on beige or white black.
- **Do** put one Lamp Red key on each screen and let everything else be grey.
- **Do** draw new frames and icons through SpriteWare's own MCP tools, 2 screen pixels per art pixel, with a black outline and a 1px bevel.
- **Do** step animation (`steps()`), and keep it to sprites, the page reveal and the scan band.
- **Do** keep hover states as a flat fill change (Hover Cream or a 8% brighten), never a colour fade.

### Don't:
- **Don't** add glow, starfields, gradients, translucent panels, blur or any ornament that is not content. (Confirmed rejection: the dark "glitter" version.)
- **Don't** use a dark theme or a second accent colour.
- **Don't** round a corner. Notch it in the art.
- **Don't** set body copy, labels or page titles in the drawn glyph face. It is wordmark-only now.
- **Don't** use white text on slate, or the theme's plum on slate.
- **Don't** introduce a native `<select>`, checkbox or scrollbar styling that breaks the frame language.
