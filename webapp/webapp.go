package webapp

import (
	. "github.com/47-11/spotifete/webapp/controller"
	"github.com/gin-gonic/gin"
	"log"
)

func Start(activeProfile string) {
	gin.SetMode(activeProfile)
	baseRouter := gin.Default()

	setupApiController(baseRouter)
	setupTemplateController(baseRouter)
	setupSpotifyController(baseRouter)

	err := baseRouter.Run(":8410")

	if err != nil {
		log.Fatalln(err.Error())
	}
}

func setupApiController(baseRouter *gin.Engine) {
	apiRouter := baseRouter.Group("/api/v1")
	apiController := new(ApiController)

	apiRouter.GET("/", apiController.Index)
	apiRouter.GET("spotify/auth/new", apiController.GetAuthUrl)
	apiRouter.GET("spotify/auth/authenticated", apiController.DidAuthSucceed)
	apiRouter.PATCH("spotify/auth/invalidate", apiController.InvalidateSessionId)
	apiRouter.GET("/sessions/:sessionId", apiController.GetSession)
	apiRouter.GET("/users/:userId", apiController.GetUser)
}

func setupTemplateController(baseRouter *gin.Engine) {
	baseRouter.LoadHTMLGlob("resources/templates/*.html")
	templateController := new(TemplateController)
	baseRouter.GET("/", templateController.Index)
	baseRouter.GET("/session/join", templateController.JoinSession)
	baseRouter.GET("/session/new", templateController.NewListeningSession)
}

func setupSpotifyController(baseRouter *gin.Engine) {
	spotifyRouter := baseRouter.Group("/spotify")
	spotifyController := new(SpotifyController)

	spotifyRouter.GET("/login", spotifyController.Login)
	spotifyRouter.GET("/callback", spotifyController.Callback)
	spotifyRouter.GET("/logout", spotifyController.Logout)
}
