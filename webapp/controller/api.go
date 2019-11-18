package controller

import (
	"github.com/47-11/spotifete/model"
	"github.com/47-11/spotifete/service"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

type ApiController struct {
	sessionService service.ListeningSessionService
	userService    service.UserService
}

func (controller ApiController) Index(c *gin.Context) {
	c.String(http.StatusOK, "SpotiFete API v1")
}

func (controller ApiController) GetActiveSessions(c *gin.Context) {
	activeSessions := controller.sessionService.GetActiveSessions()
	c.JSON(http.StatusOK, activeSessions)
}

func (controller ApiController) GetSession(c *gin.Context) {
	sessionId, err := strconv.ParseInt(c.Param("sessionId"), 0, 0)
	session, err := controller.sessionService.GetSessionByJoinId(uint(sessionId))

	if err != nil {
		if _, ok := err.(model.EntryNotFoundError); ok {
			c.String(http.StatusNotFound, err.Error())
		} else {
			c.String(http.StatusInternalServerError, err.Error())
		}
	} else {
		c.JSON(http.StatusOK, session)
	}
}

func (controller ApiController) GetUser(c *gin.Context) {
	userId, err := strconv.ParseInt(c.Param("userId"), 0, 0)
	user, err := controller.userService.GetUserById(uint(userId))

	if err != nil {
		if _, notFound := err.(model.EntryNotFoundError); notFound {
			c.String(http.StatusNotFound, err.Error())
		} else {
			c.String(http.StatusInternalServerError, err.Error())
		}
	} else {
		c.JSON(http.StatusOK, user)
	}
}
