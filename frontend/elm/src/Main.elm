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
import Types exposing (Client, ClientInstall, ClientSkill, ClientTarget, Model, Msg(..), Setup, Status, Tab(..), Tool, emptySetup, emptySkill, emptyStatus)
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
      , copied = ""
      , installNote = ""
      , installOK = False
      , skillScope = "project"
      , skillNote = ""
      , skillOK = False
      , skillText = ""
      , project = ""
      , projectChosen = False
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

        ChooseProjectFolder ->
            ( model, request { action = "choose-project", target = "" } )

        ClearProjectFolder ->
            ( model, request { action = "clear-project", target = "" } )

        GotError message ->
            ( { model | error = Just message }, Cmd.none )

        SelectTab tab ->
            -- Setup is not optional: until a path is confirmed, it is the only
            -- screen that can progress.
            if not model.setup.confirmed then
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
            ( model, request { action = "confirm-path", target = model.setup.candidate } )

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
            ( { model | client = id, copied = "", installNote = "" }, Cmd.none )

        CopySnippet id payload ->
            ( { model | copied = id }, copyToClipboard payload )


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
        , errorFromJs GotError
        ]


decodeOr : a -> Decoder a -> Decode.Value -> a
decodeOr fallback decoder value =
    Decode.decodeValue decoder value |> Result.withDefault fallback


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
