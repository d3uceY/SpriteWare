module Types exposing
    ( Client
    , ClientInstall
    , ClientSkill
    , ClientTarget
    , Model
    , Msg(..)
    , Setup
    , Status
    , Tab(..)
    , Tool
    , emptySetup
    , emptySkill
    , emptyStatus
    )

{-| The app's state, its messages, and the shapes that cross the port boundary.

Anything a port carries must be plain JSON: no custom types, no `Maybe`, no
class instances. `src/main.js` flattens the generated classes into these
records, so a rename in Go shows up as one broken field rather than a blank.

-}


type alias Status =
    { running : Bool
    , mcpPort : Int
    , url : String
    , transport : String
    , mode : String
    , bridgeConnected : Bool
    , toolCount : Int
    , lastError : String
    }


emptyStatus : Status
emptyStatus =
    { running = False
    , mcpPort = 0
    , url = ""
    , transport = ""
    , mode = ""
    , bridgeConnected = False
    , toolCount = 0
    , lastError = ""
    }


{-| Where the app is with finding Aseprite.

`candidate` is a suggestion the user has not agreed to yet; `confirmed` means
the path on disk is one they said yes to.

-}
type alias Setup =
    { confirmed : Bool
    , path : String
    , candidate : String
    , found : Bool
    , searching : Bool
    , searched : List String
    , problem : String
    }


emptySetup : Setup
emptySetup =
    { confirmed = False
    , path = ""
    , candidate = ""
    , found = False
    , searching = False
    , searched = []
    , problem = ""
    }


type alias Tool =
    { name : String
    , blurb : String
    }


{-| One assistant SpriteWare can be installed into. The snippet and the path
both come from Go, so the card and the installer can never disagree.
-}
type alias Client =
    { id : String
    , label : String
    , place : String
    , note : String
    , snippet : String
    , target : ClientTarget
    , install : ClientInstall
    , skill : ClientSkill
    }


{-| Where this assistant looks for Agent Skills. Claude Desktop takes the server
entry but has no skills folder at all, which is why Available exists.
-}
type alias ClientSkill =
    { available : Bool
    , project : String
    , global : String
    , note : String
    , globalNote : String
    }


emptySkill : ClientSkill
emptySkill =
    { available = False
    , project = ""
    , global = ""
    , note = ""
    , globalNote = ""
    }


type alias ClientTarget =
    { path : String
    , detected : Bool
    }


type alias ClientInstall =
    { can : Bool
    , why : String
    }


type Tab
    = ServerTab
    | AsepriteTab
    | ToolsTab


type alias Model =
    { status : Status
    , setup : Setup
    , tools : List Tool
    , clients : List Client
    , tab : Tab
    , pathDraft : String
    , rejected : Bool
    , client : String
    , copied : String
    , installNote : String
    , installOK : Bool
    , skillScope : String
    , skillNote : String
    , skillOK : Bool
    , skillText : String
    , ready : Bool
    , error : Maybe String
    }


type Msg
    = GotStatus Status
    | GotSetup Setup
    | GotTools (List Tool)
    | GotClients (List Client)
    | GotInstall Bool String
    | GotError String
    | SelectTab Tab
    | StartStop
    | SetMode String
    | ConfirmCandidate
    | RejectCandidate
    | TypedPath String
    | SubmitPath
    | ForgetPath
    | SelectClient String
    | CopySnippet String String
    | InstallClient String
    | SetSkillScope String
    | InstallSkill
    | CopySkill
    | GotSkill Bool String
    | GotSkillText String
