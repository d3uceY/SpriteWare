module Pixel exposing (Icon(..), Word(..), caret, icon, iconAt, mascot, word)

{-| The only pixel art in the app: the mascot, the three nav icons, the display
words, and the card frame.

Every PNG here was drawn through SpriteWare's own MCP server — `sprite op=new`,
`draw` with the grid op, `export op=png`, `sprite op=save_as` — and the
`.aseprite` files next to them in `assets/sprites` are the editable source.

The markup carries the sheet facts so `sprite.js` can drive it; that is the same
engine that will play longer clips, and the mascot already needs it: its sheet
is two frames, eyes open and eyes shut, with a per-frame timing rule.
-}

import Html exposing (Html)
import Html.Attributes as Attr


type alias Spec =
    { name : String
    , w : Int
    , h : Int
    , artPx : Float
    , label : String
    , frameMs : Int
    , timing : String
    }


{-| A CSS length from a count of art pixels. `String.fromFloat` drops the
decimal point on whole numbers, so 2 stays "2px" and 1.25 becomes "1.25px" -
which is what a tab 20% larger than the old one needs.
-}
pxOf : Float -> String
pxOf value =
    String.fromFloat value ++ "px"


{-| One sprite strip, sized by how many CSS pixels one art pixel takes.

`frameMs` and `timing` are only emitted when a sprite actually animates, so a
single-frame icon stays inert.
-}
sprite : Spec -> Html msg
sprite spec =
    let
        box =
            pxOf (toFloat spec.w * spec.artPx)

        tall =
            pxOf (toFloat spec.h * spec.artPx)

        animated =
            not (String.isEmpty spec.timing)

        frames =
            if animated then
                2

            else
                1

        strip =
            pxOf (toFloat (spec.w * frames) * spec.artPx)

        timingAttrs =
            if animated then
                [ Attr.attribute "data-frame-ms" (String.fromInt spec.frameMs)
                , Attr.attribute "data-timing" spec.timing
                ]

            else
                []
    in
    Html.span
        ([ Attr.class "sprite"
         , Attr.style "width" box
         , Attr.style "height" tall
         , Attr.attribute "data-sprite" ""
         , Attr.attribute "data-sheet" ("/sprites/" ++ spec.name ++ ".png")
         , Attr.attribute "data-frames" (String.fromInt frames)
         , Attr.attribute "data-fw" (String.fromInt spec.w)
         , Attr.attribute "data-fh" (String.fromInt spec.h)
         , Attr.attribute "data-art-px" (String.fromFloat spec.artPx)
         , Attr.attribute "aria-hidden"
            (if String.isEmpty spec.label then
                "true"

             else
                "false"
            )
         ]
            ++ timingAttrs
        )
        [ Html.img
            [ Attr.src ("/sprites/" ++ spec.name ++ ".png")
            , Attr.alt spec.label
            , Attr.style "width" strip
            , Attr.style "height" tall
            , Attr.attribute "draggable" "false"
            ]
            []
        ]


{-| The mascot: two frames, eyes open then shut. It holds open for two seconds
and blinks for ninety milliseconds, so the loop reads as a blink rather than a
twitch.
-}
mascot : Int -> Html msg
mascot artPx =
    sprite
        { name = "mascot"
        , w = 16
        , h = 16
        , artPx = toFloat artPx
        , label = "SpriteWare"
        , frameMs = 2000
        , timing = "2:90"
        }


{-| The menu caret: 7x4 art at two screen pixels each, so the chevron is drawn
rather than set. A native `<select>` brings its own arrow, and the whole point of
`caret` is that nothing here does.
-}
caret : Html msg
caret =
    sprite
        { name = "caret"
        , w = 7
        , h = 4
        , artPx = 2
        , label = ""
        , frameMs = 0
        , timing = ""
        }


type Icon
    = ServerIcon
    | BrushIcon
    | BlocksIcon
    | SearchIcon
    | FolderIcon


{-| Nav icons are 16x16 art shown in a 20px tab: one and a quarter screen
pixels per art pixel. The tab grew by a fifth, so the icons grew with it; the
browser keeps the scaling nearest-neighbour, so the pixels stay square.
-}
icon : Icon -> Html msg
icon which =
    iconAt which 1.25


{-| The same art at an explicit size, for icons that sit inside a control whose
height is already decided.
-}
iconAt : Icon -> Float -> Html msg
iconAt which artPx =
    sprite
        { name = nameOf which
        , w = 16
        , h = 16
        , artPx = artPx
        , label = ""
        , frameMs = 0
        , timing = ""
        }


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


{-| Display text, drawn on a 5x7 grid rather than set. Only the words the app
actually writes exist as art, so the type stays honest: a heading is an image
with its text as the alt.
-}
type Word
    = BrandWord
    | ServerWord
    | AsepriteWord
    | ToolsWord
    | FindWord


word : Word -> Int -> Html msg
word which artPx =
    let
        ( name, width, label ) =
            wordInfo which
    in
    sprite
        { name = name
        , w = width
        , h = 7
        , artPx = toFloat artPx
        , label = label
        , frameMs = 0
        , timing = ""
        }


wordInfo : Word -> ( String, Int, String )
wordInfo which =
    case which of
        BrandWord ->
            ( "word-brand", 59, "SpriteWare" )

        ServerWord ->
            ( "word-server", 35, "Server" )

        AsepriteWord ->
            ( "word-aseprite", 47, "Aseprite" )

        ToolsWord ->
            ( "word-tools", 29, "Tools" )

        FindWord ->
            ( "word-find", 77, "Find Aseprite" )
