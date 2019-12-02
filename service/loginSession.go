package service

import (
	"errors"
	"github.com/47-11/spotifete/database"
	"github.com/47-11/spotifete/database/model"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/gorm"
	"math/rand"
	"sync"
	"time"
)

type loginSessionService struct{}

var loginSessionServiceInstance *loginSessionService
var loginSessionServiceOnce sync.Once

func LoginSessionService() *loginSessionService {
	loginSessionServiceOnce.Do(func() {
		loginSessionServiceInstance = &loginSessionService{}
	})
	return loginSessionServiceInstance
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

func (loginSessionService) sessionIdExists(sessionId string) bool {
	var count uint
	database.Connection.Model(&model.LoginSession{}).Where(model.LoginSession{SessionId: sessionId}).Count(&count)
	return count > 0
}

func (s loginSessionService) newSessionId() string {
	for {
		b := make([]rune, 256)
		for i := range b {
			b[i] = letterRunes[rand.Intn(len(letterRunes))]
		}
		newSessionId := string(b)

		if !s.sessionIdExists(newSessionId) {
			return newSessionId
		}
	}
}

func (loginSessionService) GetSessionBySessionId(sessionId string) *model.LoginSession {
	sessions := []model.LoginSession{}
	database.Connection.Where(model.LoginSession{SessionId: sessionId}).Find(&sessions)

	if len(sessions) == 1 {
		return &sessions[0]
	}
	return nil
}

func (s loginSessionService) GetSessionFromCookie(c *gin.Context) *model.LoginSession {
	sessionId, err := c.Cookie("SESSIONID")
	if err != nil || sessionId == "" {
		// No cookie found -> Create new session id and save a new sentry with that id to the database
		return nil
	}

	// Cookie found
	session := s.GetSessionBySessionId(sessionId)
	if session != nil {
		// Sesssion found in database
		if s.IsSessionValid(*session) {
			return session
		} else {
			return nil
		}

	} else {
		// The session id from the cookie could not be found in database -> this normally should not happen and
		// could be an indicator for a malicious attack. For now just remove the cookie and return nil
		// TODO: Do something smart when this happens
		_ = s.InvalidateSession(c)
		return nil
	}
}

func (s loginSessionService) createAndSetNewSession(c *gin.Context) model.LoginSession {
	return s.createAndSetSession(c, s.newSessionId())
}

func (s loginSessionService) createAndSetSession(c *gin.Context, sessionId string) model.LoginSession {
	s.SetSessionCookie(c, sessionId)
	newLoginSession := model.LoginSession{
		Model:     gorm.Model{},
		SessionId: sessionId,
		UserId:    nil,
		Active:    true,
	}
	database.Connection.Create(&newLoginSession)

	return newLoginSession
}

func (loginSessionService) SetUserForSession(session model.LoginSession, user model.User) {
	session.UserId = &user.ID
	database.Connection.Save(session)
}

func (s loginSessionService) InvalidateSession(c *gin.Context) error {
	sessionId, err := c.Cookie("SESSIONID")
	if err == nil {
		c.SetCookie("SESSIONID", "", -1, "/", "", false, true)
		return s.InvalidateSessionBySessionId(sessionId)
	}

	return errors.New("session cookie not present")
}

func (loginSessionService) InvalidateSessionBySessionId(sessionId string) error {
	rowsAffected := database.Connection.Model(&model.LoginSession{}).Where(model.LoginSession{SessionId: sessionId}).Update("active", false).RowsAffected
	if rowsAffected > 0 {
		return nil
	} else {
		return errors.New("session id not found")
	}
}

func (loginSessionService) IsSessionValid(session model.LoginSession) bool {
	return session.Active && session.CreatedAt.AddDate(0, 1, 0).After(time.Now())
}

func (loginSessionService) SetSessionCookie(c *gin.Context, sessionId string) {
	c.SetCookie("SESSIONID", sessionId, 0, "/", "", false, true)
}
