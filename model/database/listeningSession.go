package database

import "github.com/jinzhu/gorm"

type ListeningSession struct {
	gorm.Model
	Active          bool
	OwnerId         uint
	JoinId          *string
	SpotifyPlaylist string
	Title           string
}