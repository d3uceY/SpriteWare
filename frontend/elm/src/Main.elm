port module Main exposing (main)

{-| Ports live here because Elm only allows them in the module that declares
`main`.

  - **out** `request` - a `Cmd` fired from `init`/`update`
  - **in** `statusFromJs`, `setupFromJs`, `toolsFromJs`, `errorFromJs`

Nothing in Elm knows a Promise exists; `src/main.js` turns every result and
every error into one of these messages. GSAP lives there too: Elm owns state and
structure, JavaScript owns motion.

-}

import Browser
import Json.Decode as Decode exposing (Decoder)
import Types exposing (Client, ClientInstall, ClientSkill, ClientTarget, Model, Msg(..), Settings, SettingsDraft, Setup, Status, Tab(..), Tool, Update, draftOf, emptyDraft, emptySetup, emptySettings, emptySkill, emptyStatus, emptyUpdate, parsedDraft)
import View


port request : { action : String, target : String } -> Cmd msg


port statusFromJs : (Decode.Value -> msg) -> Sub msg


port setupFromJs : (Decode.Value -> msg) -> Sub msg


port toolsFromJs : (Decode.Value -> msg) -> Sub msg


port clientsFromJs : (Decode.Value -> msg) -> Sub msg


port installFromJs : (Decode.Value -> msg) -> Sub msg


port skillFromJs : (Decode.Value -> msg) -> Sub msg


port skillTextFromJs : (String -> msg) -> Sub msg


port projectFromJs : (Decode.Value -> msg) -> Sub msg


port settingsFromJs : (Decode.Value -> msg) -> Sub msg


port savedSettingsFromJs : (Decode.Value -> msg) -> Sub msg


port versionFromJs : (String -> msg) -> Sub msg


port updateFromJs : (Decode.Value -> msg) -> Sub msg


port errorFromJs : (String -> msg) -> Sub msg


{-| The connect card copies a score-line or a JSON block. Browsers only allow
that from a real user gesture, so it has to happen in JavaScript.
-}
port copyToClipboard : String -> Cmd msg


main : Program () Model Msg
main =
    Browser.element
        { init = init
        , update = update
        , view = View.view
        , subscriptions = subscriptions
        }


init : () -> ( Model, Cmd Msg )
init _ =
    ( { status = emptyStatus
      , setup = emptySetup
      , tools = []
      , clients = []
      , tab = ServerTab
      , pathDraft = ""
      , rejected = False
      , client = "claude-desktop"
      , clientMenu = False
      , copied = ""
      , installNote = ""
      , installOK = False
      , skillScope = "project"
      , skillNote = ""
      , skillOK = False
      , skillText = ""
      , project = ""
      , projectChosen = False
      , settings = emptySettings
      , draft = emptyDraft
      , settingsNote = ""
      , settingsOK = True
      , version = ""
      , update = Nothing
      , ready = False
      , error = Nothing
      }
    , Cmd.batch
        [ request { action = "bootstrap", target = "" }
        ]
    )


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        GotStatus status ->
            ( { model | status = status, ready = True, error = Nothing }, Cmd.none )

        GotSetup setup ->
            ( { model
                | setup = setup
                , ready = True
                , error = Nothing
              }
            , Cmd.none
            )

        GotTools tools ->
            ( { model | tools = tools }, Cmd.none )

        GotClients clients ->
            -- Keep the current tab if it still exists, else the first client.
            let
                known =
                    List.any (\c -> c.id == model.client) clients

                chosen =
                    if known then
                        model.client

                    else
                        clients |> List.head |> Maybe.map .id |> Maybe.withDefault ""
            in
            ( { model | clients = clients, client = chosen }, Cmd.none )

        GotInstall ok message ->
            ( { model | installOK = ok, installNote = message }, Cmd.none )

        InstallClient id ->
            ( { model | installNote = "Installing…", installOK = True, client = id }
            , request { action = "install", target = id }
            )

        SetSkillScope scope ->
            ( { model | skillScope = scope, skillNote = "" }, Cmd.none )

        InstallSkill ->
            ( { model | skillNote = "Installing…", skillOK = True }
            , request { action = "install-skill", target = model.client ++ "|" ++ model.skillScope }
            )

        CopySkill ->
            ( { model | copied = "skill" }, copyToClipboard model.skillText )

        GotSkill ok message ->
            ( { model | skillOK = ok, skillNote = message }, Cmd.none )

        GotSkillText text ->
            ( { model | skillText = text }, Cmd.none )

        GotProject path chosen ->
            ( { model | project = path, projectChosen = chosen }, Cmd.none )

        GotVersion version ->
            ( { model | version = version }, Cmd.none )

        GotUpdate info ->
            -- Only a newer release is worth interrupting for: "up to date" and
            -- "could not check" are silence, not a dialog.
            ( { model
                | update =
                    if info.available then
                        Just info

                    else
                        Nothing
              }
            , Cmd.none
            )

        DismissUpdate ->
            ( { model | update = Nothing }, Cmd.none )

        OpenDownloads url ->
            ( model, request { action = "open-downloads", target = url } )

        ChooseProjectFolder ->
            ( model, request { action = "choose-project", target = "" } )

        GotError message ->
            ( { model | error = Just message }, Cmd.none )

        SelectTab tab ->
            -- Setup is not optional: until a path is confirmed, it is the only
            -- screen that can progress. Settings is the exception, because a port
            -- that is taken is exactly why the setup screen may be stuck.
            if not model.setup.confirmed && tab /= AsepriteTab && tab /= SettingsTab then
                ( { model | tab = AsepriteTab }, Cmd.none )

            else
                ( { model | tab = tab }, Cmd.none )

        StartStop ->
            ( model
            , request
                { action =
                    if model.status.running then
                        "stop"

                    else
                        "start"
                , target = ""
                }
            )

        SetMode mode ->
            ( model, request { action = "mode", target = mode } )

        ConfirmCandidate ->
            -- Close the dialog first. If the path turns out to be unusable, the
            -- reason belongs on the card; a modal that stays up would hide the
            -- only thing that explains why nothing happened.
            ( { model | rejected = True }
            , request { action = "confirm-path", target = model.setup.candidate }
            )

        BrowsePath ->
            ( { model | error = Nothing }, request { action = "browse-aseprite", target = "" } )

        FindPath ->
            -- Searching again is also the way back from "not this one", so the
            -- dismissal is cleared here rather than on a timer.
            ( { model | rejected = False, error = Nothing }
            , request { action = "find-aseprite", target = "" }
            )

        RejectCandidate ->
            ( { model | rejected = True }, Cmd.none )

        TypedPath text ->
            ( { model | pathDraft = text, error = Nothing }, Cmd.none )

        SubmitPath ->
            ( model, request { action = "confirm-path", target = model.pathDraft } )

        ForgetPath ->
            ( { model | rejected = False, pathDraft = "" }
            , request { action = "forget-path", target = "" }
            )

        SelectClient id ->
            ( { model | client = id, copied = "", installNote = "", clientMenu = False }
            , Cmd.none
            )

        ToggleClientMenu ->
            ( { model | clientMenu = not model.clientMenu }, Cmd.none )

        CloseClientMenu ->
            ( { model | clientMenu = False }, Cmd.none )

        CopySnippet id payload ->
            ( { model | copied = id }, copyToClipboard payload )

        GotSettings settings ->
            -- A fetch is the only thing that resets the draft, so a status push
            -- in the middle of typing cannot undo it.
            ( { model
                | settings = settings
                , draft = draftOf settings
                , settingsNote = ""
                , settingsOK = True
              }
            , Cmd.none
            )

        TypedMcpPort text ->
            let
                draft =
                    model.draft
            in
            ( { model
                | draft = { draft | mcpPort = digitsOnly text }
                , settingsNote = ""
              }
            , Cmd.none
            )

        TypedBridgePort text ->
            let
                draft =
                    model.draft
            in
            ( { model
                | draft = { draft | bridgePort = digitsOnly text }
                , settingsNote = ""
              }
            , Cmd.none
            )

        DraftTransport transport ->
            let
                draft =
                    model.draft
            in
            ( { model | draft = { draft | transport = transport }, settingsNote = "" }, Cmd.none )

        DraftStartOnLaunch ->
            let
                draft =
                    model.draft
            in
            ( { model
                | draft = { draft | startOnLaunch = not draft.startOnLaunch }
                , settingsNote = ""
              }
            , Cmd.none
            )

        DiscardSettings ->
            ( { model
                | draft = draftOf model.settings
                , settingsNote = ""
                , settingsOK = True
                , error = Nothing
              }
            , Cmd.none
            )

        SaveSettings ->
            case parsedDraft model.draft of
                Nothing ->
                    ( { model
                        | settingsOK = False
                        , settingsNote = "Both ports have to be whole numbers from 1 to 65535, and they have to differ."
                      }
                    , Cmd.none
                    )

                Just next ->
                    ( { model | settingsOK = True, settingsNote = "Saving…", error = Nothing }
                    , request { action = "save-settings", target = encodeSettings next }
                    )

        GotSettingsSaved ok message settings ->
            if ok then
                ( { model
                    | settings = settings
                    , draft = draftOf settings
                    , settingsOK = True
                    , settingsNote = message
                  }
                , Cmd.none
                )

            else
                -- The draft stays as typed: the reason is about the numbers in
                -- the fields, and the user has to be able to fix them.
                ( { model | settingsOK = False, settingsNote = message }, Cmd.none )


subscriptions : Model -> Sub Msg
subscriptions _ =
    Sub.batch
        [ statusFromJs (decodeOr emptyStatus statusDecoder >> GotStatus)
        , setupFromJs (decodeOr emptySetup setupDecoder >> GotSetup)
        , toolsFromJs (decodeOr [] toolsDecoder >> GotTools)
        , clientsFromJs (decodeOr [] clientsDecoder >> GotClients)
        , installFromJs (decodeOr ( False, "" ) installDecoder >> \( ok, message ) -> GotInstall ok message)
        , skillFromJs (decodeOr ( False, "" ) installDecoder >> \( ok, message ) -> GotSkill ok message)
        , skillTextFromJs GotSkillText
        , projectFromJs (decodeOr ( "", False ) projectDecoder >> \( path, chosen ) -> GotProject path chosen)
        , settingsFromJs (decodeOr emptySettings settingsDecoder >> GotSettings)
        , savedSettingsFromJs (decodeOr ( False, "", emptySettings ) savedSettingsDecoder >> \( ok, message, settings ) -> GotSettingsSaved ok message settings)
        , versionFromJs GotVersion
        , updateFromJs (decodeOr emptyUpdate updateDecoder >> GotUpdate)
        , errorFromJs GotError
        ]


decodeOr : a -> Decoder a -> Decode.Value -> a
decodeOr fallback decoder value =
    Decode.decodeValue decoder value |> Result.withDefault fallback


{-| A port field holds nothing but digits, so typing cannot produce a value the
server refuses for a reason the user did not intend.
-}
digitsOnly : String -> String
digitsOnly text =
    String.filter Char.isDigit text |> String.left 5


{-| The request port carries one string, the way `install-skill` carries two
values. -}
encodeSettings : Settings -> String
encodeSettings settings =
    String.join "|"
        [ String.fromInt settings.mcpPort
        , settings.transport
        , String.fromInt settings.bridgePort
        , if settings.startOnLaunch then
            "1"

          else
            "0"
        ]


statusDecoder : Decoder Status
statusDecoder =
    Decode.map8 Status
        (Decode.field "running" Decode.bool)
        (Decode.field "port" Decode.int)
        (Decode.field "url" Decode.string)
        (Decode.field "transport" Decode.string)
        (Decode.field "mode" Decode.string)
        (Decode.field "bridgeConnected" Decode.bool)
        (Decode.field "toolCount" Decode.int)
        (Decode.oneOf [ Decode.field "lastError" Decode.string, Decode.succeed "" ])


setupDecoder : Decoder Setup
setupDecoder =
    Decode.map7 Setup
        (Decode.field "confirmed" Decode.bool)
        (Decode.field "path" Decode.string)
        (Decode.field "candidate" Decode.string)
        (Decode.field "found" Decode.bool)
        (Decode.oneOf [ Decode.field "searching" Decode.bool, Decode.succeed False ])
        (Decode.oneOf [ Decode.field "searched" (Decode.list Decode.string), Decode.succeed [] ])
        (Decode.oneOf [ Decode.field "problem" Decode.string, Decode.succeed "" ])


toolsDecoder : Decoder (List Tool)
toolsDecoder =
    Decode.list
        (Decode.map2 Tool
            (Decode.field "name" Decode.string)
            (Decode.field "blurb" Decode.string)
        )


clientsDecoder : Decoder (List Client)
clientsDecoder =
    Decode.list
        (Decode.map8 Client
            (Decode.field "id" Decode.string)
            (Decode.field "label" Decode.string)
            (Decode.field "place" Decode.string)
            (Decode.field "note" Decode.string)
            (Decode.field "snippet" Decode.string)
            (Decode.field "target" clientTargetDecoder)
            (Decode.field "install" clientInstallDecoder)
            (Decode.oneOf [ Decode.field "skill" clientSkillDecoder, Decode.succeed emptySkill ])
        )


clientSkillDecoder : Decoder ClientSkill
clientSkillDecoder =
    Decode.map5 ClientSkill
        (Decode.oneOf [ Decode.field "available" Decode.bool, Decode.succeed False ])
        (Decode.oneOf [ Decode.field "project" Decode.string, Decode.succeed "" ])
        (Decode.oneOf [ Decode.field "global" Decode.string, Decode.succeed "" ])
        (Decode.oneOf [ Decode.field "note" Decode.string, Decode.succeed "" ])
        (Decode.oneOf [ Decode.field "globalNote" Decode.string, Decode.succeed "" ])


clientTargetDecoder : Decoder ClientTarget
clientTargetDecoder =
    Decode.map2 ClientTarget
        (Decode.oneOf [ Decode.field "path" Decode.string, Decode.succeed "" ])
        (Decode.oneOf [ Decode.field "detected" Decode.bool, Decode.succeed False ])


clientInstallDecoder : Decoder ClientInstall
clientInstallDecoder =
    Decode.map2 ClientInstall
        (Decode.oneOf [ Decode.field "can" Decode.bool, Decode.succeed False ])
        (Decode.oneOf [ Decode.field "why" Decode.string, Decode.succeed "" ])


installDecoder : Decoder ( Bool, String )
installDecoder =
    Decode.map2 Tuple.pair
        (Decode.oneOf [ Decode.field "ok" Decode.bool, Decode.succeed False ])
        (Decode.oneOf [ Decode.field "message" Decode.string, Decode.succeed "" ])


projectDecoder : Decoder ( String, Bool )
projectDecoder =
    Decode.map2 Tuple.pair
        (Decode.oneOf [ Decode.field "path" Decode.string, Decode.succeed "" ])
        (Decode.oneOf [ Decode.field "chosen" Decode.bool, Decode.succeed False ])


settingsDecoder : Decoder Settings
settingsDecoder =
    Decode.map4 Settings
        (Decode.field "port" Decode.int)
        (Decode.oneOf [ Decode.field "transport" Decode.string, Decode.succeed "streamable-http" ])
        (Decode.field "bridgePort" Decode.int)
        (Decode.oneOf [ Decode.field "startMcpOnLaunch" Decode.bool, Decode.succeed True ])


savedSettingsDecoder : Decoder ( Bool, String, Settings )
savedSettingsDecoder =
    Decode.map3 (\ok message settings -> ( ok, message, settings ))
        (Decode.oneOf [ Decode.field "ok" Decode.bool, Decode.succeed False ])
        (Decode.oneOf [ Decode.field "message" Decode.string, Decode.succeed "" ])
        (Decode.field "settings" settingsDecoder)


updateDecoder : Decoder Update
updateDecoder =
    Decode.map5 Update
        (Decode.oneOf [ Decode.field "current" Decode.string, Decode.succeed "" ])
        (Decode.oneOf [ Decode.field "latest" Decode.string, Decode.succeed "" ])
        (Decode.oneOf [ Decode.field "available" Decode.bool, Decode.succeed False ])
        (Decode.oneOf [ Decode.field "url" Decode.string, Decode.succeed "" ])
        (Decode.oneOf [ Decode.field "notes" Decode.string, Decode.succeed "" ])
