module View exposing (view)

import Html exposing (Html)
import Html.Attributes as Attr
import Html.Events as Events
import Json.Decode as Decode
import Types exposing (Model, Msg(..))


view : Model -> Html Msg
view model =
    Html.div [ Attr.class "shell" ]
        [ header model
        , Html.main_ [ Attr.class "cards" ]
            [ runtimeCard model
            , echoCard model
            ]
        , footer model
        ]


header : Model -> Html Msg
header model =
    Html.header [ Attr.class "top" ]
        [ Html.div [ Attr.class "brand" ]
            [ Html.span [ Attr.class "brand__mark" ] [ Html.text "SW" ]
            , Html.div []
                [ Html.h1 [ Attr.class "brand__title" ] [ Html.text "SpriteWare" ]
                , Html.p [ Attr.class "brand__sub" ] [ Html.text "Wails v3 + Elm starter" ]
                ]
            ]
        , status model
        ]


status : Model -> Html Msg
status model =
    let
        ( className, label ) =
            case model.error of
                Just _ ->
                    ( "status status--bad", "backend error" )

                Nothing ->
                    if String.isEmpty model.info.go then
                        ( "status status--wait", "connecting" )

                    else
                        ( "status status--ok", "connected" )
    in
    Html.span [ Attr.class className ] [ Html.text label ]


runtimeCard : Model -> Html Msg
runtimeCard model =
    Html.section [ Attr.class "card" ]
        [ Html.h2 [ Attr.class "card__title" ] [ Html.text "Runtime" ]
        , Html.p [ Attr.class "card__hint" ]
            [ Html.text "Read live from Go through the generated binding." ]
        , Html.div [ Attr.class "rows" ]
            [ row "App" model.info.version
            , row "Go" model.info.go
            , row "Wails" model.info.wails
            , row "Platform" model.info.platform
            , row "Elm" "0.19.3"
            ]
        ]


echoCard : Model -> Html Msg
echoCard model =
    Html.section [ Attr.class "card" ]
        [ Html.h2 [ Attr.class "card__title" ] [ Html.text "Round trip" ]
        , Html.p [ Attr.class "card__hint" ]
            [ Html.text "Port out, binding call, port back in." ]
        , Html.div [ Attr.class "form" ]
            [ Html.input
                [ Attr.class "input"
                , Attr.type_ "text"
                , Attr.placeholder "say something to Go"
                , Attr.value model.input
                , Attr.autofocus True
                , Attr.spellcheck False
                , Events.onInput Typed
                , Events.on "keydown" enterKey
                ]
                []
            , Html.button [ Attr.class "btn", Events.onClick Submit ]
                [ Html.text "Send" ]
            ]
        , feedback model
        ]


{-| Enter submits. A decode failure simply ignores the key, so every other
keypress behaves normally.
-}
enterKey : Decode.Decoder Msg
enterKey =
    Decode.field "key" Decode.string
        |> Decode.andThen
            (\key ->
                if key == "Enter" then
                    Decode.succeed Submit

                else
                    Decode.fail "not Enter"
            )


feedback : Model -> Html Msg
feedback model =
    case ( model.error, model.reply ) of
        ( Just message, _ ) ->
            Html.p [ Attr.class "feedback feedback--bad" ] [ Html.text message ]

        ( Nothing, "" ) ->
            Html.p [ Attr.class "feedback" ] [ Html.text "Waiting for a reply." ]

        ( Nothing, reply ) ->
            Html.p [ Attr.class "feedback feedback--ok" ] [ Html.text reply ]


footer : Model -> Html Msg
footer model =
    Html.footer [ Attr.class "bottom" ]
        [ Html.span [ Attr.class "bottom__note" ]
            [ Html.text "Pushed from Go once a second - the other direction." ]
        , Html.span [ Attr.class "bottom__clock" ] [ Html.text (orDash model.clock) ]
        ]


row : String -> String -> Html msg
row label value =
    Html.div [ Attr.class "row" ]
        [ Html.span [ Attr.class "row__key" ] [ Html.text label ]
        , Html.span [ Attr.class "row__val" ] [ Html.text (orDash value) ]
        ]


orDash : String -> String
orDash value =
    if String.isEmpty value then
        "—"

    else
        value
