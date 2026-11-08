module Types exposing
    ( Client
    , ClientInstall
    , ClientSkill
    , ClientTarget
    , Model
    , Msg(..)
    , Settings
    , SettingsDraft
    , Setup
    , Status
    , Tab(..)
    , Tool
    , Update
    , draftOf
    , emptyDraft
    , emptySetup
    , emptySettings
    , emptySkill
    , emptyStatus
    , emptyUpdate
    , parsedDraft
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


{-| What GitHub says about the newest release. `available` is decided in Go, so
the window never has to compare version strings itself.

-}
type alias Update =
    { current : String
    , latest : String
    , available : Bool
    , url : String
    , notes : String
    }


emptyUpdate : Update
emptyUpdate =
    { current = ""
    , latest = ""
    , available = False
    , url = ""
    , notes = ""
    }


{-| The four numbers and switches a user can change. Everything else in
`config.json` (the Aseprite path, the project folder, the Lua switch) is set by
the screen that owns it.
-}
type alias Settings =
    { mcpPort : Int
    , transport : String
    , bridgePort : Int
    , startOnLaunch : Bool
    }


emptySettings : Settings
emptySettings =
    { mcpPort = 9933
    , transport = "streamable-http"
    , bridgePort = 9934
    , startOnLaunch = True
    }


{-| The same four as typed: ports stay text so a half-typed "99" is not a
number yet, and saving is what turns them into one.
-}
type alias SettingsDraft =
    { mcpPort : String
    , transport : String
    , bridgePort : String
    , startOnLaunch : Bool
    }


emptyDraft : SettingsDraft
emptyDraft =
    { mcpPort = ""
    , transport = "streamable-http"
    , bridgePort = ""
    , startOnLaunch = True
    }


{-| The saved settings as the form shows them, so "is this edited" is one
comparison rather than four.
-}
draftOf : Settings -> SettingsDraft
draftOf settings =
    { mcpPort = String.fromInt settings.mcpPort
    , transport = settings.transport
    , bridgePort = String.fromInt settings.bridgePort
    , startOnLaunch = settings.startOnLaunch
    }


{-| Typed text as a value, or nothing while it is still unusable. Go checks
again before writing: this only keeps the button quiet until the numbers add up.
-}
parsedDraft : SettingsDraft -> Maybe Settings
parsedDraft draft =
    case ( String.toInt draft.mcpPort, String.toInt draft.bridgePort ) of
        ( Just mcpPort, Just bridgePort ) ->
            if mcpPort < 1 || mcpPort > 65535 || bridgePort < 1 || bridgePort > 65535 then
                Nothing

            else if mcpPort == bridgePort then
                Nothing

            else
                Just
                    { mcpPort = mcpPort
                    , transport = draft.transport
                    , bridgePort = bridgePort
                    , startOnLaunch = draft.startOnLaunch
                    }

        _ ->
            Nothing


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
    | SettingsTab


type alias Model =
    { status : Status
    , setup : Setup
    , tools : List Tool
    , clients : List Client
    , tab : Tab
    , pathDraft : String
    , rejected : Bool
    , client : String
    , clientMenu : Bool
    , copied : String
    , installNote : String
    , installOK : Bool
    , skillScope : String
    , skillNote : String
    , skillOK : Bool
    , skillText : String
    , project : String
    , projectChosen : Bool
    , settings : Settings
    , draft : SettingsDraft
    , settingsNote : String
    , settingsOK : Bool
    , version : String
    , update : Maybe Update
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
    | BrowsePath
    | FindPath
    | TypedPath String
    | SubmitPath
    | ForgetPath
    | SelectClient String
    | ToggleClientMenu
    | CloseClientMenu
    | CopySnippet String String
    | InstallClient String
    | SetSkillScope String
    | InstallSkill
    | CopySkill
    | GotSkill Bool String
    | GotSkillText String
    | ChooseProjectFolder
    | GotProject String Bool
    | GotSettings Settings
    | TypedMcpPort String
    | TypedBridgePort String
    | DraftTransport String
    | DraftStartOnLaunch
    | SaveSettings
    | DiscardSettings
    | GotSettingsSaved Bool String Settings
    | GotVersion String
    | GotUpdate Update
    | DismissUpdate
    | OpenDownloads String
