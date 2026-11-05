port module Main exposing (main)

{-| Ports live here because Elm only allows them in the module that declares
`main`. Two directions:

  - **out** `request` - a `Cmd`, fired from `init`/`update`
  - **in** the four `*FromJs` ports - subscriptions, wired in `subscriptions`

Nothing in Elm knows a Promise exists; `src/main.js` turns every result and
every error into one of these messages.

-}
import Browser
import Types exposing (Info, Model, Msg(..), emptyInfo)
import View


{-| The single outbound channel. `action` picks a binding in `main.js`,
`target` is its (optional) argument.
-}
port request : { action : String, target : String } -> Cmd msg


port infoFromJs : (Info -> msg) -> Sub msg


port replyFromJs : (String -> msg) -> Sub msg


port tickFromJs : (String -> msg) -> Sub msg


port errorFromJs : (String -> msg) -> Sub msg


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
    ( { info = emptyInfo
      , input = "hello"
      , reply = ""
      , clock = ""
      , error = Nothing
      }
    , request { action = "info", target = "" }
    )


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        GotInfo info ->
            ( { model | info = info, error = Nothing }, Cmd.none )

        GotReply reply ->
            ( { model | reply = reply, error = Nothing }, Cmd.none )

        GotTick clock ->
            ( { model | clock = clock }, Cmd.none )

        GotError message ->
            ( { model | error = Just message }, Cmd.none )

        Typed text ->
            ( { model | input = text, error = Nothing }, Cmd.none )

        Submit ->
            ( model, request { action = "echo", target = model.input } )


subscriptions : Model -> Sub Msg
subscriptions _ =
    Sub.batch
        [ infoFromJs GotInfo
        , replyFromJs GotReply
        , tickFromJs GotTick
        , errorFromJs GotError
        ]
