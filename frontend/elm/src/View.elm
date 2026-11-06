module View exposing (view)

import Html exposing (Html)
import Html.Attributes as Attr
import Html.Events as Events
import Json.Decode as Decode
import Pixel exposing (Icon(..), Word(..), icon)
import Types exposing (Client, Model, Msg(..), Setup, Status, Tab(..), Tool)


{-| One shell, four screens. The rail is always there; the stage changes.
-}
view : Model -> Html Msg
view model =
    Html.div
        [ Attr.class "app"
        , Attr.attribute "data-view" (viewName model)
        ]
        [ rail model
        , Html.main_ [ Attr.class "stage" ] [ stage model ]
        ]


viewName : Model -> String
viewName model =
    if not model.setup.confirmed then
        "setup"

    else
        case model.tab of
            ServerTab ->
                "server"

            AsepriteTab ->
                "aseprite"

            ToolsTab ->
                "tools"


stage : Model -> Html Msg
stage model =
    if not model.setup.confirmed then
        setupPage model

    else
        case model.tab of
            ServerTab ->
                serverPage model

            AsepriteTab ->
                asepritePage model

            ToolsTab ->
                toolsPage model


{-| The rail: the mark, three destinations, and the connection state.

Every label names the screen it opens; the pixel art is the only decoration.
-}
rail : Model -> Html Msg
rail model =
    Html.aside [ Attr.class "rail" ]
        [ Html.div [ Attr.class "brand" ]
            [ Pixel.mascot 3
            , Pixel.word BrandWord 2
            ]
        , Html.nav [ Attr.class "nav" ]
            [ navItem model ServerTab ServerIcon "Server"
            , navItem model AsepriteTab BrushIcon "Aseprite"
            , navItem model ToolsTab BlocksIcon "Tools"
            ]
        , Html.footer [ Attr.class "rail__foot" ]
            [ statePlate model ]
        ]


navItem : Model -> Tab -> Icon -> String -> Html Msg
navItem model tab which label =
    let
        active =
            if not model.setup.confirmed then
                tab == AsepriteTab

            else
                model.tab == tab

        locked =
            not model.setup.confirmed && tab /= AsepriteTab
    in
    Html.button
        [ Attr.class
            (if active then
                "navi navi--on"

             else
                "navi"
            )
        , Attr.disabled locked
        , Attr.attribute "aria-current"
            (if active then
                "page"

             else
                "false"
            )
        , Events.onClick (SelectTab tab)
        ]
        [ icon which 1
        , Html.span [ Attr.class "navi__label" ] [ Html.text label ]
        ]


{-| The state plate in the rail foot, the way vectile shows its model state:
a dot, a word, and the detail.
-}
statePlate : Model -> Html Msg
statePlate model =
    let
        ( modifier, word ) =
            if not model.setup.confirmed then
                ( "todo", "Needs Aseprite" )

            else if model.status.running then
                ( "live", "Listening" )

            else
                ( "idle", "Stopped" )
    in
    Html.div [ Attr.class "state" ]
        [ Html.span [ Attr.class ("state__dot state__dot--" ++ modifier) ] []
        , Html.span [ Attr.class "state__word" ] [ Html.text word ]
        ]


{-| Page heading: a drawn title and one line of real text that says what the
page is for. The title is art, so its words live in the alt attribute.
-}
pageHead : Pixel.Word -> String -> Html msg
pageHead title lead =
    Html.header [ Attr.class "page__head" ]
        [ Html.h1 [ Attr.class "page__title" ] [ Pixel.word title 3 ]
        , Html.p [ Attr.class "page__lead" ] [ Html.text lead ]
        ]


{-| First run: ask about the binary. The suggestion is never assumed.
-}
setupPage : Model -> Html Msg
setupPage model =
    Html.div [ Attr.class "page" ]
        [ pageHead FindWord
            "Point SpriteWare at Aseprite once. It remembers the path."
        , if model.setup.found && not model.rejected then
            suggestion model

          else
            manualPath model
        , problemNote model
        ]


suggestion : Model -> Html Msg
suggestion model =
    Html.section [ Attr.class "card" ]
        [ Html.h2 [ Attr.class "card__title" ] [ Html.text "Found Aseprite" ]
        , Html.code [ Attr.class "code" ] [ Html.text model.setup.candidate ]
        , Html.div [ Attr.class "row row--end" ]
            [ Html.button [ Attr.class "btn btn--ghost", Events.onClick RejectCandidate ]
                [ Html.text "Choose another" ]
            , Html.button [ Attr.class "btn", Events.onClick ConfirmCandidate ]
                [ Html.text "Use this" ]
            ]
        ]


manualPath : Model -> Html Msg
manualPath model =
    Html.section [ Attr.class "card" ]
        [ Html.h2 [ Attr.class "card__title" ] [ Html.text "Where is Aseprite?" ]
        , Html.label [ Attr.class "label", Attr.for "aseprite-path" ]
            [ Html.text "Path" ]
        , Html.input
            [ Attr.id "aseprite-path"
            , Attr.class "field"
            , Attr.type_ "text"
            , Attr.placeholder "C:\\Program Files\\Aseprite\\aseprite.exe"
            , Attr.value model.pathDraft
            , Attr.spellcheck False
            , Attr.autofocus True
            , Events.onInput TypedPath
            , Events.on "keydown" enterKey
            ]
            []
        , Html.div [ Attr.class "row" ]
            [ Html.button [ Attr.class "btn", Events.onClick SubmitPath ]
                [ Html.text "Use path" ]
            , if model.setup.found then
                Html.button [ Attr.class "btn btn--ghost", Events.onClick RejectCandidate ]
                    [ Html.text "Use found path" ]

              else
                Html.text ""
            ]
        , searchHint model
        ]


searchHint : Model -> Html Msg
searchHint model =
    if model.setup.searching then
        Html.p [ Attr.class "hint" ]
            [ Html.text "Searching install folders." ]

    else if List.isEmpty model.setup.searched then
        Html.text ""

    else
        Html.p [ Attr.class "hint" ]
            [ Html.text
                ("Searched PATH, install folders, and "
                    ++ String.fromInt (List.length model.setup.searched)
                    ++ " nearby folders."
                )
            ]


enterKey : Decode.Decoder Msg
enterKey =
    Decode.field "key" Decode.string
        |> Decode.andThen
            (\key ->
                if key == "Enter" then
                    Decode.succeed SubmitPath

                else
                    Decode.fail "not Enter"
            )


problemNote : Model -> Html Msg
problemNote model =
    case model.error of
        Just message ->
            Html.p [ Attr.class "note note--bad", Attr.attribute "role" "alert" ] [ Html.text message ]

        Nothing ->
            if String.isEmpty model.setup.problem then
                Html.text ""

            else
                Html.p [ Attr.class "note note--bad", Attr.attribute "role" "alert" ]
                    [ Html.text model.setup.problem ]


{-| The server page: is it listening, where, and how an assistant connects.
-}
serverPage : Model -> Html Msg
serverPage model =
    let
        status =
            model.status
    in
    Html.div [ Attr.class "page" ]
        [ pageHead ServerWord
            "Loopback only. Start it, then connect an assistant."
        , Html.section [ Attr.class "card" ]
            [ Html.div [ Attr.class "statusline" ]
                [ Html.span
                    [ Attr.class
                        ("statusline__dot statusline__dot--"
                            ++ (if status.running then
                                    "live"

                                else
                                    "idle"
                               )
                        )
                    ]
                    []
                , Html.span [ Attr.class "statusline__word" ]
                    [ Html.text
                        (if status.running then
                            "Listening"

                         else
                            "Stopped"
                        )
                    ]
                , Html.span [ Attr.class "statusline__meta" ]
                    [ Html.text (transportLabel status.transport ++ " · " ++ modeLabel status.mode) ]
                ]
            , Html.div [ Attr.class "url" ]
                [ Html.code [ Attr.class "url__text" ] [ Html.text (serverUrl status) ]
                , copyButton model "url" (serverUrl status)
                ]
            , Html.div [ Attr.class "row row--between" ]
                [ Html.button
                    [ Attr.class
                        (if status.running then
                            "btn btn--ghost"

                         else
                            "btn"
                        )
                    , Events.onClick StartStop
                    ]
                    [ Html.text
                        (if status.running then
                            "Stop"
                         else
                            "Start server"
                        )
                    ]
                , Html.div [ Attr.class "seg" ]
                    [ segButton status "headless" "Background"
                    , segButton status "live" "Editor window"
                    ]
                ]
            , problemNote model
            ]
        , connectCard model
        , skillCard model
        ]


{-| The snippet card: pick an assistant, read where the config goes, then copy
or install. Every string on it comes from Go, so the text you copy and the
object Install writes are the same description.
-}
connectCard : Model -> Html Msg
connectCard model =
    let
        client =
            currentClient model
    in
    Html.section [ Attr.class "card" ]
        [ Html.div [ Attr.class "card__head" ]
            [ Html.h2 [ Attr.class "card__title" ] [ Html.text "Connect an assistant" ]
            ]
        , clientPicker model
        , Html.p [ Attr.class "card__lead" ] [ Html.text client.place ]
        , Html.pre [ Attr.class "snippet__code" ] [ Html.code [] [ Html.text client.snippet ] ]
        , Html.div [ Attr.class "row row--between" ]
            [ Html.span [ Attr.class "target" ]
                [ Html.span
                    [ Attr.class
                        ("target__dot target__dot--"
                            ++ (if client.target.detected then
                                    "found"

                                else
                                    "missing"
                               )
                        )
                    ]
                    []
                , Html.text client.target.path
                ]
            , Html.div [ Attr.class "row" ]
                [ copyButton model client.id client.snippet
                , Html.button
                    [ Attr.class "btn btn--ghost"
                    , Attr.disabled (not client.install.can)
                    , Events.onClick (InstallClient client.id)
                    ]
                    [ Html.text "Install" ]
                ]
            ]
        , supportLine model client
        ]


{-| One line under the buttons, never two: the result if there is one, then why
an install is impossible, then the agent's own gotcha.
-}
supportLine : Model -> Client -> Html Msg
supportLine model client =
    if not (String.isEmpty model.installNote) then
        Html.p
            [ Attr.class
                ("hint "
                    ++ (if model.installOK then
                            "hint--ok"

                        else
                            "hint--bad"
                       )
                )
            , Attr.attribute "role" "status"
            ]
            [ Html.text model.installNote ]

    else if not client.install.can && not (String.isEmpty client.install.why) then
        Html.p [ Attr.class "hint" ] [ Html.text client.install.why ]

    else if String.isEmpty client.note then
        Html.text ""

    else
        Html.p [ Attr.class "hint" ] [ Html.text client.note ]


{-| The skill card: the same assistant tabs, but a skills folder instead of a
config file. A skill is a folder holding a SKILL.md, so this one copies a file —
there is no config to merge and nothing of the user's to preserve.
-}
skillCard : Model -> Html Msg
skillCard model =
    let
        client =
            currentClient model

        chosen =
            if model.skillScope == "global" then
                client.skill.global

            else
                client.skill.project
    in
    Html.section [ Attr.class "card" ]
        [ Html.div [ Attr.class "card__head" ]
            [ Html.h2 [ Attr.class "card__title" ]
                [ Html.text ("Teach " ++ client.label ++ " the tools") ]
            , Html.p [ Attr.class "card__lead" ]
                [ Html.text "Ten workflows and the rules they share." ]
            ]
        , if not client.skill.available then
            Html.div [ Attr.class "row row--between" ]
                [ Html.p [ Attr.class "hint" ]
                    [ Html.text "No skills folder. Copy SKILL.md and add it yourself." ]
                , Html.button
                    [ Attr.class "btn btn--link"
                    , Events.onClick CopySkill
                    , Attr.disabled (String.isEmpty model.skillText)
                    ]
                    [ Html.text
                        (if model.copied == "skill" then
                            "Copied"

                         else
                            "Copy SKILL.md"
                        )
                    ]
                ]

          else
            Html.div [ Attr.class "skill" ]
                [ Html.div [ Attr.class "seg" ]
                    [ scopeButton model "project" "This project"
                    , scopeButton model "global" "Every project"
                    ]
                , Html.div [ Attr.class "url" ]
                    [ Html.code [ Attr.class "url__text" ] [ Html.text chosen ]
                    , Html.button
                        [ Attr.class "btn btn--link"
                        , Events.onClick CopySkill
                        , Attr.disabled (String.isEmpty model.skillText)
                        ]
                        [ Html.text
                            (if model.copied == "skill" then
                                "Copied"

                             else
                                "Copy SKILL.md"
                            )
                        ]
                    ]
                , Html.div [ Attr.class "row row--end" ]
                    [ Html.button
                        [ Attr.class "btn btn--ghost"
                        , Events.onClick InstallSkill
                        ]
                        [ Html.text "Install skill" ]
                    ]
                , skillLine model client
                ]
        ]


{-| One line, same rule as the card above. Kiro's extra step wins over the
rest because it is the only one that blocks the skill from loading.
-}
skillLine : Model -> Client -> Html Msg
skillLine model client =
    if not (String.isEmpty model.skillNote) then
        Html.p
            [ Attr.class
                ("hint "
                    ++ (if model.skillOK then
                            "hint--ok"

                        else
                            "hint--bad"
                       )
                )
            , Attr.attribute "role" "status"
            ]
            [ Html.text model.skillNote ]

    else
        Html.p [ Attr.class "hint" ] [ Html.text (skillNoteFor model client) ]


{-| The caveat belongs to the scope: Claude Code loads personal skills from
`~/.claude/skills`, but Cowork and cloud sessions skip them.
-}
skillNoteFor : Model -> Client -> String
skillNoteFor model client =
    if model.skillScope == "global" then
        client.skill.globalNote

    else
        client.skill.note


scopeButton : Model -> String -> String -> Html Msg
scopeButton model value label =
    Html.button
        [ Attr.class
            (if model.skillScope == value then
                "seg__btn seg__btn--on"

             else
                "seg__btn"
            )
        , Events.onClick (SetSkillScope value)
        ]
        [ Html.text label ]


currentClient : Model -> Client
currentClient model =
    model.clients
        |> List.filter (\c -> c.id == model.client)
        |> List.head
        |> Maybe.withDefault fallbackClient


fallbackClient : Client
fallbackClient =
    { id = ""
    , label = ""
    , place = ""
    , note = ""
    , snippet = ""
    , target = { path = "", detected = False }
    , install = { can = False, why = "" }
    , skill = { available = False, project = "", global = "", note = "", globalNote = "" }
    }


clientTab : Model -> Client -> Html Msg
clientTab model client =
    Html.button
        [ Attr.class
            (if model.client == client.id then
                "tabs__tab tabs__tab--on"

             else
                "tabs__tab"
            )
        , Events.onClick (SelectClient client.id)
        ]
        [ Html.text client.label ]


{-| Eleven agents do not fit as pills without turning into a wall, so the
choice is a native picker: one row, every option, and the keyboard and screen
reader get the platform behaviour for free.
-}
clientPicker : Model -> Html Msg
clientPicker model =
    Html.div [ Attr.class "picker" ]
        [ Html.label [ Attr.class "label", Attr.for "assistant" ] [ Html.text "Assistant" ]
        , Html.select
            [ Attr.id "assistant"
            , Attr.class "select"
            , Events.on "change"
                (Decode.at [ "target", "value" ] Decode.string
                    |> Decode.map SelectClient
                )
            ]
            (List.map (clientOption model) model.clients)
        ]


clientOption : Model -> Client -> Html Msg
clientOption model client =
    Html.option
        [ Attr.value client.id
        , Attr.selected (model.client == client.id)
        ]
        [ Html.text client.label ]


serverUrl : Status -> String
serverUrl status =
    if not (String.isEmpty status.url) then
        status.url

    else
        "http://127.0.0.1:"
            ++ String.fromInt status.mcpPort
            ++ (if status.transport == "sse" then
                    "/sse"

                else
                    "/mcp"
               )


copyButton : Model -> String -> String -> Html Msg
copyButton model id payload =
    Html.button
        [ Attr.class "btn btn--link"
        , Events.onClick (CopySnippet id payload)
        ]
        [ Html.text
            (if model.copied == id then
                "Copied"

             else
                "Copy"
            )
        ]


segButton : Status -> String -> String -> Html Msg
segButton status value label =
    let
        active =
            status.mode == value
    in
    Html.button
        [ Attr.class
            (if active then
                "seg__btn seg__btn--on"

             else
                "seg__btn"
            )
        , Attr.disabled active
        , Events.onClick (SetMode value)
        ]
        [ Html.text label ]


asepritePage : Model -> Html Msg
asepritePage model =
    Html.div [ Attr.class "page" ]
        [ pageHead AsepriteWord "The program SpriteWare drives. Change it if Aseprite moves."
        , Html.section [ Attr.class "card" ]
            [ Html.span [ Attr.class "label" ] [ Html.text "Program" ]
            , Html.code [ Attr.class "code" ] [ Html.text model.setup.path ]
            , Html.div [ Attr.class "row" ]
                [ Html.button [ Attr.class "btn btn--ghost", Events.onClick ForgetPath ]
                    [ Html.text "Change path" ]
                ]
            , problemNote model
            ]
        ]


toolsPage : Model -> Html Msg
toolsPage model =
    Html.div [ Attr.class "page" ]
        [ pageHead ToolsWord
            (String.fromInt (List.length model.tools)
                ++ " tools, each a group of related commands."
            )
        , Html.section [ Attr.class "card card--flush" ]
            [ Html.ul [ Attr.class "tools" ] (List.map toolRow model.tools) ]
        ]


toolRow : Tool -> Html Msg
toolRow tool =
    Html.li [ Attr.class "tool" ]
        [ Html.code [ Attr.class "tool__name" ] [ Html.text tool.name ]
        , Html.span [ Attr.class "tool__blurb" ] [ Html.text tool.blurb ]
        ]


transportLabel : String -> String
transportLabel transport =
    if transport == "sse" then
        "SSE"

    else
        "Streamable HTTP"


modeLabel : String -> String
modeLabel mode =
    case mode of
        "live" ->
            "Editor window"

        "headless" ->
            "Background"

        _ ->
            "—"
