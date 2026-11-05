module Types exposing (Info, Model, Msg(..), emptyInfo)


{-| The app's state, its messages, and the shapes that cross the port boundary.

Anything a port carries must be plain JSON: no custom types, no `Maybe`, no
class instances. Flatten in `src/main.js`, rebuild the shape here.

-}
type alias Info =
    { version : String
    , go : String
    , wails : String
    , platform : String
    }


{-| `Info` before Go has answered - every field blank, so the view can tell
"still connecting" apart from "connected".
-}
emptyInfo : Info
emptyInfo =
    { version = ""
    , go = ""
    , wails = ""
    , platform = ""
    }


type alias Model =
    { info : Info
    , input : String
    , reply : String
    , clock : String
    , error : Maybe String
    }


type Msg
    = GotInfo Info
    | GotReply String
    | GotTick String
    | GotError String
    | Typed String
    | Submit
