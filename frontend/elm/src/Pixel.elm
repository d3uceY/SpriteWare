module Pixel exposing (Icon(..), caret, icon, led, mascot, text)

{-| The pixel art in the app. Every sheet in `public/sprites` was drawn through
SpriteWare's own MCP server, and the `.aseprite` sources sit in
`assets/sprites`.

Nothing here knows a size or a frame count. A sprite is a class name, and
`style.css` owns the geometry (`--w`, `--h`, `--n`, `--px`) and the animation:
frames are stepped with `background-position`, so a sprite needs no script, no
re-diffing and no clock of its own.

-}

import Html exposing (Html)
import Html.Attributes as Attr


sprite : String -> Html msg
sprite name =
    Html.span
        [ Attr.class ("px px--" ++ name)
        , Attr.attribute "aria-hidden" "true"
        ]
        []


{-| The app mark: the idle loop (nine idle frames, then a blink). `live` plays
the smiling sheet while the server is listening; hover adds the blushing one.
-}
mascot : Bool -> Html msg
mascot live =
    Html.span
        [ Attr.class
            ("px px--mascot"
                ++ (if live then
                        " px--mascot-live"

                    else
                        ""
                   )
            )
        , Attr.attribute "aria-hidden" "true"
        ]
        []


caret : Html msg
caret =
    sprite "caret"


{-| The state lamp: `live` pulses, `todo` blinks amber, anything else is dark.
-}
led : String -> Html msg
led state =
    Html.span
        [ Attr.class ("px px--led px--led-" ++ state)
        , Attr.attribute "aria-hidden" "true"
        ]
        []


type Icon
    = ServerIcon
    | BrushIcon
    | BlocksIcon
    | SearchIcon
    | FolderIcon
    | PlayIcon
    | StopIcon


{-| The nav icons are two-frame sheets that only play while their tab is open.
-}
icon : Icon -> Html msg
icon which =
    sprite (nameOf which)


nameOf : Icon -> String
nameOf which =
    case which of
        ServerIcon ->
            "nav-server"

        BrushIcon ->
            "nav-aseprite"

        BlocksIcon ->
            "nav-tools"

        SearchIcon ->
            "icon-search"

        FolderIcon ->
            "icon-folder"

        PlayIcon ->
            "icon-play"

        StopIcon ->
            "icon-stop"


{-| Display text, set one drawn glyph at a time from the `glyphs` sheet.

The classes pick the tone and the size (`--px` in CSS), so the same word can be
a masthead or a ghost numeral. The words live in `aria-label`: the glyphs
themselves are decoration.

-}
text : String -> String -> Html msg
text classes content =
    Html.span
        [ Attr.class ("glyphs " ++ classes)
        , Attr.attribute "role" "img"
        , Attr.attribute "aria-label" content
        ]
        (content |> String.toUpper |> String.toList |> List.map glyph)


glyph : Char -> Html msg
glyph character =
    Html.span
        [ Attr.class "glyph"
        , Attr.attribute "style" ("--i:" ++ String.fromInt (glyphIndex character))
        ]
        []


{-| The order the sheet was drawn in. A character with no glyph is a space.
-}
glyphOrder : String
glyphOrder =
    "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 .:/-"


glyphIndex : Char -> Int
glyphIndex character =
    String.indexes (String.fromChar character) glyphOrder
        |> List.head
        |> Maybe.withDefault 36
