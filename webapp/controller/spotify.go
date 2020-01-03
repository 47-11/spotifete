package controller

import (
	"github.com/47-11/spotifete/service"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
)

type SpotifyController struct{}

func (controller SpotifyController) Login(c *gin.Context) {
	authUrl, _ := service.SpotifyService().NewAuthUrl()
	c.Redirect(http.StatusTemporaryRedirect, authUrl)
}

func (controller SpotifyController) Callback(c *gin.Context) {
	// Set user and token in session and redirect back to index
	state := c.Request.FormValue("state")

	// Check that this state exists and was not used in a callback before
	session := service.LoginSessionService().GetSessionBySessionId(state)
	if session == nil {
		c.String(http.StatusUnauthorized, "Unknown state.")
		return
	}

	if session.UserId != nil {
		c.String(http.StatusUnauthorized, "State has already been used.")
		return
	}

	if !service.LoginSessionService().IsSessionValid(*session) {
		c.String(http.StatusUnauthorized, "Session is no longer valid.")
		return
	}

	// Fetch the token
	token, err := service.SpotifyService().Authenticator.Token(state, c.Request)
	if err != nil {
		c.String(http.StatusUnauthorized, "Could not get token: "+err.Error())
		log.Println(err.Error())
		return
	}

	// Get the spotify user for the token
	client := service.SpotifyService().Authenticator.NewClient(token)
	spotifyUser, err := client.CurrentUser()
	if err != nil {
		c.String(http.StatusInternalServerError, "Could not get current spotify user: "+err.Error())
		log.Println(err.Error())
		return
	}

	// Cache the created client
	service.SpotifyService().Clients[spotifyUser.ID] = &client

	// Get or create the database entry for the current user
	user := service.UserService().GetOrCreateUser(spotifyUser)
	service.UserService().SetToken(*user, *token)

	// Associate user with current session
	service.LoginSessionService().SetUserForSession(*session, *user)

	// Set or update session cookie
	service.LoginSessionService().SetSessionCookie(c, session.SessionId)

	c.Redirect(http.StatusTemporaryRedirect, "/")
}

func (controller SpotifyController) Logout(c *gin.Context) {
	_ = service.LoginSessionService().InvalidateSession(c)

	c.Redirect(http.StatusTemporaryRedirect, "/")
}
