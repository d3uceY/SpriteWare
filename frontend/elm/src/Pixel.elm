module Pixel exposing (Icon(..), Word(..), icon, mascot, word)

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
    , artPx : Int
    , label : String
    , frameMs : Int
    , timing : String
    }


{-| One sprite strip, sized by how many CSS pixels one art pixel takes.

`frameMs` and `timing` are only emitted when a sprite actually animates, so a
single-frame icon stays inert.
-}
sprite : Spec -> Html msg
sprite spec =
    let
        box =
            String.fromInt (spec.w * spec.artPx) ++ "px"

        tall =
            String.fromInt (spec.h * spec.artPx) ++ "px"

        animated =
            not (String.isEmpty spec.timing)

        frames =
            if animated then
                2

            else
                1

        strip =
            String.fromInt (spec.w * frames * spec.artPx) ++ "px"

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
         , Attr.attribute "data-art-px" (String.fromInt spec.artPx)
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
        , artPx = artPx
        , label = "SpriteWare"
        , frameMs = 2000
        , timing = "2:90"
        }


type Icon
    = ServerIcon
    | BrushIcon
    | BlocksIcon


{-| Nav icons keep their own colours at every state; the active tab is already
said by the row's background and label.
-}
icon : Icon -> Int -> Html msg
icon which artPx =
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
        , artPx = artPx
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
