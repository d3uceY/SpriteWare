module View exposing (view)

import Html exposing (Html)
import Html.Attributes as Attr
import Html.Events as Events
import Json.Decode as Decode
import Pixel exposing (Icon(..), icon)
import Types exposing (Client, Model, Msg(..), Status, Tab(..), Tool, draftOf, parsedDraft)


{-| One window, four screens. The chrome is a strip along the top and the
stage below it changes; every screen is a tab, the way an editor shows
open documents.
-}
view : Model -> Html Msg
view model =
    Html.div
        [ Attr.class "app"
        , Attr.attribute "data-view" (viewName model)
        ]
        [ chrome model
        , Html.main_
            [ Attr.class "stage"
            , Attr.attribute "role" "tabpanel"
            , Attr.attribute "aria-labelledby" (tabId (shownTab model))
            ]
            [ stage model ]
        , confirmDialog model
        , updateDialog model
        ]


{-| Which tab the stage is showing. Until a path is confirmed, setup borrows
the Aseprite tab because that is the screen that can make progress. Settings is
reachable anyway: a port that is taken is a reason setup cannot finish.
-}
shownTab : Model -> Tab
shownTab model =
    if model.setup.confirmed || model.tab == SettingsTab then
        model.tab

    else
        AsepriteTab


viewName : Model -> String
viewName model =
    if not model.setup.confirmed && model.tab /= SettingsTab then
        "setup"

    else
        case model.tab of
            ServerTab ->
                "server"

            AsepriteTab ->
                "aseprite"

            ToolsTab ->
                "tools"

            SettingsTab ->
                "settings"


stage : Model -> Html Msg
stage model =
    if not model.setup.confirmed && model.tab /= SettingsTab then
        setupPage model

    else
        case model.tab of
            ServerTab ->
                serverPage model

            AsepriteTab ->
                asepritePage model

            ToolsTab ->
                toolsPage model

            SettingsTab ->
                settingsPage model


{-| The strip along the top: the build and connection state on a quiet line,
then the mascot and the tabs on the rail below.

The mascot is the wordmark now. It sits at the left of the tab strip, on the
rail, with the tabs pushed to the right. The tabs are drawn frames (`tab.png`
for a closed chip, `tab-on.png` for the open one) with real text inside. The
open frame has no bottom edge, which is what lets the selected tab flow into
the stage below.
-}
chrome : Model -> Html Msg
chrome model =
    Html.header [ Attr.class "chrome" ]
        [ Html.div [ Attr.class "topbar" ]
            [ Html.div [ Attr.class "topbar__end" ]
                [ versionBadge model
                , statePlate model
                ]
            ]
        , Html.div [ Attr.class "tabstrip" ]
            [ Html.div [ Attr.class "tabstrip__mark" ]
                [ Pixel.mascot model.status.running ]
            , Html.nav
                [ Attr.class "tabs"
                , Attr.attribute "role" "tablist"
                , Attr.attribute "aria-label" "Screens"
                ]
                [ tabButton model ServerTab ServerIcon "Server"
                , tabButton model AsepriteTab BrushIcon "Aseprite"
                , tabButton model ToolsTab BlocksIcon "Tools"
                , tabButton model SettingsTab SettingsIcon "Settings"
                ]
            ]
        ]


tabId : Tab -> String
tabId tab =
    case tab of
        ServerTab ->
            "tab-server"

        AsepriteTab ->
            "tab-aseprite"

        ToolsTab ->
            "tab-tools"

        SettingsTab ->
            "tab-settings"


tabButton : Model -> Tab -> Icon -> String -> Html Msg
tabButton model tab which label =
    let
        active =
            shownTab model == tab

        locked =
            not model.setup.confirmed && tab /= AsepriteTab && tab /= SettingsTab
    in
    Html.button
        [ Attr.class
            (if active then
                "tab tab--on"

             else
                "tab"
            )
        , Attr.id (tabId tab)
        , Attr.attribute "role" "tab"
        , Attr.attribute "aria-selected"
            (if active then
                "true"

             else
                "false"
            )
        , Attr.disabled locked
        , Events.onClick (SelectTab tab)
        ]
        [ icon which
        , Html.span [ Attr.class "tab__label" ] [ Html.text label ]
        ]


{-| Stamped into the binary at release time, so Go is the only source. Empty
outside the window (browser preview), where there is nothing to show.
-}
versionBadge : Model -> Html Msg
versionBadge model =
    if model.version == "" then
        Html.text ""

    else
        Html.span [ Attr.class "version" ]
            [ Html.text ("v" ++ model.version) ]


{-| The connection state at the right edge of the strip: a dot and a word.
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
    Html.div [ Attr.class "state", Attr.attribute "role" "status" ]
        [ Pixel.led modifier
        , Html.span [ Attr.class "state__word" ] [ Html.text word ]
        ]


{-| Page head: the title set in the display face, one span per character so the
view change can print it a glyph at a time, and one line of intent.
-}
pageHead : String -> String -> Html msg
pageHead title lead =
    Html.header [ Attr.class "page__head" ]
        [ Html.h1
            [ Attr.class "page__title"
            , Attr.attribute "aria-label" title
            ]
            (List.map titleChar (String.toList title))
        , Html.p [ Attr.class "page__lead" ] [ Html.text lead ]
        ]


titleChar : Char -> Html msg
titleChar ch =
    Html.span [ Attr.class "page__char" ] [ Html.text (String.fromChar ch) ]


{-| First run: ask about the binary.
-}
setupPage : Model -> Html Msg
setupPage model =
    Html.div [ Attr.class "page" ]
        [ pageHead "Find Aseprite"
            "Point SpriteWare at Aseprite once. It remembers the path."
        , Html.section
            [ Attr.class
                (if model.setup.searching then
                    "card card--scan"

                 else
                    "card"
                )
            ]
            [ Html.div [ Attr.class "card__head" ]
                [ Html.h2 [ Attr.class "card__title" ] [ Html.text "Where is Aseprite?" ]
                , Html.p [ Attr.class "card__lead" ]
                    [ Html.text "Let SpriteWare sweep the disk for it, or point at the program yourself." ]
                ]
            , pathField model
            , Html.div [ Attr.class "row row--between" ]
                [ findButton model
                , Html.button
                    [ Attr.class "btn btn--ghost"
                    , Attr.disabled (String.isEmpty (String.trim model.pathDraft))
                    , Events.onClick SubmitPath
                    ]
                    [ Html.text "Use this path" ]
                ]
            , searchHint model
            , problemNote model
            ]
        ]


{-| One field with two ways in: type a path, or open the platform's own file
picker from the button sitting inside the same frame. The button belongs in the
field because it answers the same question; a separate row of actions would put
three buttons where the user has two decisions.
-}
pathField : Model -> Html Msg
pathField model =
    Html.div [ Attr.class "pathfield" ]
        [ Html.input
            [ Attr.id "aseprite-path"
            , Attr.class "pathfield__input"
            , Attr.type_ "text"
            , Attr.placeholder "C:\\Program Files\\Aseprite\\aseprite.exe"
            , Attr.value model.pathDraft
            , Attr.spellcheck False
            , Attr.autofocus True
            , Events.onInput TypedPath
            , Events.on "keydown" enterKey
            ]
            []
        , Html.button
            [ Attr.class "pathfield__browse"
            , Attr.type_ "button"
            , Attr.title "Browse for the Aseprite program"
            , Events.onClick BrowsePath
            ]
            [ icon FolderIcon
            , Html.span [] [ Html.text "Browse" ]
            ]
        ]


{-| The one filled action on this screen: it is the answer most people want,
and it is also the only thing here that takes a while.
-}
findButton : Model -> Html Msg
findButton model =
    Html.button
        [ Attr.class "btn btn--icon"
        , Attr.disabled model.setup.searching
        , Events.onClick FindPath
        ]
        [ icon SearchIcon
        , Html.span []
            [ Html.text
                (if model.setup.searching then
                    "Searching…"

                 else
                    "Find it"
                )
            ]
        ]


{-| Whether the "found Aseprite" prompt is up. Only until it is confirmed: the
candidate stays set afterwards so the Aseprite screen can show it, and a prompt
about a path that is already saved would never stop asking.
-}
setupDialogOpen : Model -> Bool
setupDialogOpen model =
    model.setup.found && not model.setup.confirmed && not model.rejected


{-| The path a search returns is a guess, so it arrives in a dialog that names
it and asks. `alertdialog` rather than `dialog`: it interrupts to ask for an
answer, and a screen reader should say so.
-}
confirmDialog : Model -> Html Msg
confirmDialog model =
    if setupDialogOpen model then
        Html.div [ Attr.class "dialog__wrap", Attr.attribute "data-dialog" "" ]
            [ Html.div
                [ Attr.class "dialog__scrim"
                , Events.onClick RejectCandidate
                ]
                []
            , Html.div
                [ Attr.class "dialog"
                , Attr.attribute "role" "alertdialog"
                , Attr.attribute "aria-modal" "true"
                , Attr.attribute "aria-labelledby" "found-title"
                ]
                [ Html.div [ Attr.class "dialog__head" ]
                    [ Html.div [ Attr.class "dialog__mark" ] [ Pixel.mascot model.status.running ]
                    , Html.div [ Attr.class "dialog__headline" ]
                        [ Html.span [ Attr.class "label" ] [ Html.text "Found Aseprite" ]
                        , Html.h2 [ Attr.id "found-title", Attr.class "dialog__title" ]
                            [ Html.text "Is this the one?" ]
                        ]
                    ]
                , Html.code [ Attr.class "dialog__path" ] [ Html.text model.setup.candidate ]
                , Html.p [ Attr.class "dialog__lead" ]
                    [ Html.text
                        "SpriteWare will run it for every job. Nothing is saved until you say yes."
                    ]
                , Html.div [ Attr.class "row row--end" ]
                    [ Html.button
                        [ Attr.class "btn btn--ghost"
                        , Attr.attribute "data-dialog-cancel" ""
                        , Events.onClick RejectCandidate
                        ]
                        [ Html.text "Not this one" ]
                    , Html.button
                        [ Attr.class "btn"
                        , Attr.attribute "data-dialog-confirm" ""
                        , Events.onClick ConfirmCandidate
                        ]
                        [ Html.text "Use this path" ]
                    ]
                ]
            ]

    else
        Html.text ""


{-| A newer release exists. It is not a question and it can wait, so this is a
`dialog` and the answer that carries on is the quiet one. The setup prompt wins
when both would be up: a copy that cannot find Aseprite has a bigger problem
than being one version behind.
-}
updateDialog : Model -> Html Msg
updateDialog model =
    case model.update of
        Just update ->
            if setupDialogOpen model then
                Html.text ""

            else
                Html.div [ Attr.class "dialog__wrap", Attr.attribute "data-dialog" "" ]
                    [ Html.div
                        [ Attr.class "dialog__scrim"
                        , Events.onClick DismissUpdate
                        ]
                        []
                    , Html.div
                        [ Attr.class "dialog"
                        , Attr.attribute "role" "dialog"
                        , Attr.attribute "aria-modal" "true"
                        , Attr.attribute "aria-labelledby" "update-title"
                        ]
                        [ Html.div [ Attr.class "dialog__head" ]
                            [ Html.div [ Attr.class "dialog__mark" ] [ Pixel.mascot model.status.running ]
                            , Html.div [ Attr.class "dialog__headline" ]
                                [ Html.h2 [ Attr.id "update-title", Attr.class "dialog__title" ]
                                    [ Html.text ("v" ++ update.latest ++ " is out") ]
                                ]
                            ]
                        , updateLead update
                        , updateNotes update
                        , Html.div [ Attr.class "row row--end" ]
                            [ Html.button
                                [ Attr.class "btn btn--ghost"
                                , Attr.attribute "data-dialog-cancel" ""
                                , Events.onClick DismissUpdate
                                ]
                                [ Html.text "Later" ]
                            , Html.button
                                [ Attr.class "btn"
                                , Attr.attribute "data-dialog-confirm" ""
                                , Events.onClick (OpenDownloads update.url)
                                ]
                                [ Html.text "Download" ]
                            ]
                        ]
                    ]

        Nothing ->
            Html.text ""


updateLead : Types.Update -> Html Msg
updateLead update =
    if update.current == "" then
        Html.text ""

    else
        Html.p [ Attr.class "dialog__lead" ] [ Html.text ("You have v" ++ update.current) ]


updateNotes : Types.Update -> Html Msg
updateNotes update =
    if update.notes == "" then
        Html.text ""

    else
        Html.p [ Attr.class "dialog__notes" ] [ Html.text update.notes ]


searchHint : Model -> Html Msg
searchHint model =
    if model.setup.searching then
        Html.p [ Attr.class "hint" ]
            [ Html.text "Sweeping PATH, the usual install folders, and the folders around them." ]

    else if List.isEmpty model.setup.searched then
        Html.p [ Attr.class "hint" ]
            [ Html.text "Nothing is saved until you confirm a path." ]

    else
        Html.p [ Attr.class "hint" ]
            [ Html.text
                ("Looked at PATH, the install folders, and "
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
    Html.div [ Attr.class "page" ]
        [ pageHead "Server"
            "Connect an assistant."
        , startCard model
        , connectCard model
        , skillCard model
        ]


startCard : Model -> Html Msg
startCard model =
    let
        status =
            model.status
    in
    Html.section [ Attr.class "card" ]
        [ Html.div [ Attr.class "statusline" ]
            [ Pixel.led
                (if status.running then
                    "live"

                 else
                    "idle"
                )
            , Html.span [ Attr.class "statusline__word" ]
                [ Html.text
                    (if status.running then
                        "Listening"

                     else
                        "Stopped"
                    )
                ]
            , Html.span [ Attr.class "statusline__meta" ]
                [ Html.text (transportLabel status.transport) ]
            ]
        , Html.div
            [ Attr.class
                ("url"
                    ++ (if status.running then
                            " url--on"

                        else
                            ""
                       )
                )
            ]
            [ Html.code [ Attr.class "url__text" ] [ Html.text (serverUrl status) ]
            , copyButton model "url" (serverUrl status)
            ]
        , Html.div [ Attr.class "row row--between" ]
            [ Html.button
                [ Attr.class
                    (if status.running then
                        "btn btn--ghost btn--icon"

                     else
                        "btn btn--icon"
                    )
                , Events.onClick StartStop
                ]
                [ icon
                    (if status.running then
                        StopIcon

                     else
                        PlayIcon
                    )
                , Html.span []
                    [ Html.text
                        (if status.running then
                            "Stop"

                         else
                            "Start server"
                        )
                    ]
                ]
            , Html.div [ Attr.class "seg" ]
                [ segChoice (status.mode == "headless") "Background" (SetMode "headless")
                , segChoice (status.mode == "live") "Editor window" (SetMode "live")
                ]
            ]
        , bridgeNote model
        , serverProblem model
        ]

{-| Live mode is only useful with Aseprite on the other end, and "Listening"
looks the same whether or not anything is attached. Say which it is.
-}
bridgeNote : Model -> Html Msg
bridgeNote model =
    if model.status.mode /= "live" then
        Html.text ""

    else if model.status.bridgeConnected then
        Html.p [ Attr.class "bridge bridge--on" ]
            [ Html.text "Aseprite is connected. Tool calls draw in the open window." ]

    else
        Html.p [ Attr.class "bridge bridge--waiting" ]
            [ Html.text "Waiting for Aseprite: open it with the SpriteWare extension enabled." ]


{-| Why nothing is listening, whichever path reported it, with the way out of
it beside the reason. A start at launch has no caller left to report to, so that
failure only exists as `lastError`.
-}
serverProblem : Model -> Html Msg
serverProblem model =
    case problemText model of
        "" ->
            Html.text ""

        message ->
            Html.div [ Attr.class "row row--between" ]
                [ Html.p [ Attr.class "note note--bad", Attr.attribute "role" "alert" ]
                    [ Html.text message ]
                , Html.button
                    [ Attr.class "btn btn--ghost"
                    , Events.onClick (SelectTab SettingsTab)
                    ]
                    [ Html.text "Change port" ]
                ]


problemText : Model -> String
problemText model =
    case model.error of
        Just message ->
            message

        Nothing ->
            if not (String.isEmpty model.status.lastError) then
                model.status.lastError

            else
                model.setup.problem


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
                [ Html.text "A SKILL.md with the coordinate and frame rules." ]
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
                [ projectRow model
                , Html.div [ Attr.class "seg" ]
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


{-| Which folder "This project" means. Left to itself it is the folder
SpriteWare was started from, which from a shortcut is the install directory, so
the chosen folder is shown and can be changed - and until there is one, the
card says so instead of printing a folder nobody picked.
-}
projectRow : Model -> Html Msg
projectRow model =
    Html.div [ Attr.class "project" ]
        [ Html.div [ Attr.class "row row--between" ]
            [ Html.span [ Attr.class "target" ]
                [ Html.text
                    (if model.projectChosen then
                        model.project

                     else
                        "Project folder: not chosen"
                    )
                ]
            , Html.div [ Attr.class "row" ]
                [ Html.button
                    [ Attr.class "btn btn--link"
                    , Events.onClick ChooseProjectFolder
                    ]
                    [ Html.text
                        (if model.projectChosen then
                            "Change folder"

                         else
                            "Choose folder"
                        )
                    ]
                ]
            ]
        , if model.projectChosen then
            Html.text ""

          else
            Html.p [ Attr.class "hint" ]
                [ Html.text "The path below is relative to it. Pick the folder you are working in to install there." ]
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


{-| Eleven agents do not fit as pills without turning into a wall, so the choice
is one row that opens a list. A native `<select>` cannot be used: the platform
draws its popup, arrow and all, so the one control the app could not make look
like itself was the one with the most options. This is a listbox instead - a
drawn trigger, a drawn panel, and a button per option, which also means the
keyboard can tab through them.
-}
clientPicker : Model -> Html Msg
clientPicker model =
    let
        client =
            currentClient model
    in
    -- No visible "Assistant" label: the trigger already names the agent, and a
    -- label beside a dropdown that says what it is spells out the obvious. The
    -- accessible name lives on the button instead.
    Html.div [ Attr.class "picker" ]
        [ Html.div [ Attr.class "menu" ]
            (Html.button
                [ Attr.id "assistant"
                , Attr.class "menu__trigger"
                , Attr.attribute "aria-haspopup" "listbox"
                , Attr.attribute "aria-label" "Assistant"
                , Attr.attribute "aria-expanded"
                    (if model.clientMenu then
                        "true"

                     else
                        "false"
                    )
                , Events.onClick ToggleClientMenu
                , Events.on "keydown" escapeKey
                ]
                [ Html.span [ Attr.class "menu__value" ] [ Html.text client.label ]
                , Html.span [ Attr.class "menu__caret" ] [ Pixel.caret ]
                ]
                :: menuItems model
            )
        ]


{-| The panel and the click-catcher behind it. The scrim is a fixed, transparent
sheet: clicking anywhere outside the list closes it without a port or a document
listener.
-}
menuItems : Model -> List (Html Msg)
menuItems model =
    if model.clientMenu then
        [ Html.div [ Attr.class "menu__scrim", Events.onClick CloseClientMenu ] []
        , Html.ul [ Attr.class "menu__list", Attr.attribute "role" "listbox" ]
            (List.map (clientOption model) model.clients)
        ]

    else
        []


clientOption : Model -> Client -> Html Msg
clientOption model client =
    let
        chosen =
            model.client == client.id
    in
    Html.li
        [ Attr.class "menu__item"
        , Attr.attribute "role" "presentation"
        ]
        [ Html.button
            [ Attr.class
                (if chosen then
                    "menu__option menu__option--on"

                 else
                    "menu__option"
                )
            , Attr.attribute "role" "option"
            , Attr.attribute "aria-selected"
                (if chosen then
                    "true"

                 else
                    "false"
                )
            , Events.onClick (SelectClient client.id)
            ]
            [ Html.text client.label ]
        ]


escapeKey : Decode.Decoder Msg
escapeKey =
    Decode.field "key" Decode.string
        |> Decode.andThen
            (\key ->
                if key == "Escape" then
                    Decode.succeed CloseClientMenu

                else
                    Decode.fail "not Escape"
            )


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


segChoice : Bool -> String -> Msg -> Html Msg
segChoice active label onClickMsg =
    Html.button
        [ Attr.class
            (if active then
                "seg__btn seg__btn--on"

             else
                "seg__btn"
            )
        , Attr.disabled active
        , Events.onClick onClickMsg
        ]
        [ Html.text label ]


{-| The four values the app had to guess for the user. Each one fixes something
they cannot see otherwise: where assistants connect, how they talk, where the
extension dials in, and whether the endpoint is up before anyone asks.
-}
settingsPage : Model -> Html Msg
settingsPage model =
    let
        draft =
            model.draft

        ready =
            parsedDraft draft /= Nothing
    in
    Html.div [ Attr.class "page" ]
        [ pageHead "Settings"
            "Ports and startup."
        , Html.section [ Attr.class "card" ]
            [ Html.div [ Attr.class "card__head" ]
                [ Html.h2 [ Attr.class "card__title" ] [ Html.text "MCP server" ]
                , Html.p [ Attr.class "card__lead" ]
                    [ Html.text "Assistant connection." ]
                ]
            , portField "settings-mcp-port" "Port" draft.mcpPort TypedMcpPort
            , Html.div [ Attr.class "row" ]
                [ Html.div [ Attr.class "seg" ]
                    [ segChoice (draft.transport == "streamable-http")
                        "Streamable HTTP"
                        (DraftTransport "streamable-http")
                    , segChoice (draft.transport == "sse") "SSE" (DraftTransport "sse")
                    ]
                ]
            , Html.p [ Attr.class "hint" ]
                [ Html.text "Restarts the server. Reinstall assistants afterwards." ]
            ]
        , Html.section [ Attr.class "card" ]
            [ Html.div [ Attr.class "card__head" ]
                [ Html.h2 [ Attr.class "card__title" ] [ Html.text "Aseprite bridge" ]
                , Html.p [ Attr.class "card__lead" ]
                    [ Html.text "Live mode only." ]
                ]
            , portField "settings-bridge-port" "Port" draft.bridgePort TypedBridgePort
            , if ready then
                Html.text ""

              else
                Html.p [ Attr.class "note note--bad", Attr.attribute "role" "alert" ]
                    [ Html.text "Ports must be 1-65535 and differ." ]
            , Html.p [ Attr.class "hint" ]
                [ Html.text "Rewrites the extension. Restart Aseprite." ]
            ]
        , Html.section [ Attr.class "card" ]
            [ Html.div [ Attr.class "card__head" ]
                [ Html.h2 [ Attr.class "card__title" ] [ Html.text "At launch" ] ]
            , Html.div [ Attr.class "row" ]
                [ Html.button
                    [ Attr.class
                        (if draft.startOnLaunch then
                            "switch switch--on"

                         else
                            "switch"
                        )
                    , Attr.attribute "role" "switch"
                    , Attr.attribute "aria-checked"
                        (if draft.startOnLaunch then
                            "true"

                         else
                            "false"
                        )
                    , Events.onClick DraftStartOnLaunch
                    ]
                    [ Pixel.switch draft.startOnLaunch
                    , Html.span [ Attr.class "switch__label" ]
                        [ Html.text
                            (if draft.startOnLaunch then
                                "Start with the app"

                             else
                                "Start manually"
                            )
                        ]
                    ]
                ]
            , Html.p [ Attr.class "hint" ]
                [ Html.text "Applies on the next launch." ]
            ]
        , Html.div [ Attr.class "row row--between" ]
            [ Html.button
                [ Attr.class "btn btn--ghost"
                , Attr.disabled (draft == draftOf model.settings)
                , Events.onClick DiscardSettings
                ]
                [ Html.text "Discard" ]
            , Html.button
                [ Attr.class "btn"
                , Attr.disabled (not ready)
                , Events.onClick SaveSettings
                ]
                [ Html.text "Save" ]
            ]
        , settingsNote model
        , problemNote model
        ]


{-| A port in a well: the host is fixed, so it is printed rather than typed. -}
portField : String -> String -> String -> (String -> Msg) -> Html Msg
portField id label value onInput =
    Html.div [ Attr.class "field" ]
        [ Html.label [ Attr.class "field__label", Attr.for id ] [ Html.text label ]
        , Html.div [ Attr.class "numfield" ]
            [ Html.span [ Attr.class "numfield__prefix" ] [ Html.text "127.0.0.1:" ]
            , Html.input
                [ Attr.id id
                , Attr.class "numfield__input"
                , Attr.type_ "text"
                , Attr.attribute "inputmode" "numeric"
                , Attr.attribute "autocomplete" "off"
                , Attr.value value
                , Attr.spellcheck False
                , Events.onInput onInput
                ]
                []
            ]
        ]


{-| The answer to a save: what changed, or why nothing did. -}
settingsNote : Model -> Html Msg
settingsNote model =
    if String.isEmpty model.settingsNote then
        Html.text ""

    else
        Html.p
            [ Attr.class
                ("hint "
                    ++ (if model.settingsOK then
                            "hint--ok"

                        else
                            "hint--bad"
                       )
                )
            , Attr.attribute "role" "status"
            ]
            [ Html.text model.settingsNote ]


asepritePage : Model -> Html Msg
asepritePage model =
    Html.div [ Attr.class "page" ]
        [ pageHead "Aseprite"
            "The program SpriteWare drives. Change it if Aseprite moves."
        , Html.section [ Attr.class "card" ]
            [ Html.div [ Attr.class "statusline" ]
                [ Pixel.led "live"
                , Html.span [ Attr.class "label" ] [ Html.text "Program" ]
                ]
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
    let
        count =
            List.length model.tools
    in
    Html.div [ Attr.class "page" ]
        [ pageHead "Tools"
            (String.fromInt count
                ++ (if count == 1 then
                        " tool. Each one groups related commands."

                    else
                        " tools, each a group of related commands."
                   )
            )
        , if count == 0 then
            Html.section [ Attr.class "card" ]
                [ Html.p [ Attr.class "empty" ]
                    [ Html.text "No tools registered. Restart the app if this stays empty." ]
                ]

          else
            Html.ul [ Attr.class "tools" ] (List.map toolRow model.tools)
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
