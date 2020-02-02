package service

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/47-11/spotifete/database"
	. "github.com/47-11/spotifete/model/database"
	dto "github.com/47-11/spotifete/model/dto"
	"github.com/getsentry/sentry-go"
	"github.com/google/logger"
	"github.com/jinzhu/gorm"
	qrcode "github.com/skip2/go-qrcode"
	"github.com/zmb3/spotify"
	"image/jpeg"
	"math/rand"
	"sync"
	"time"
)

type listeningSessionService struct {
	numberRunes []rune
}

var listeningSessionServiceInstance *listeningSessionService
var listeningSessionServiceOnce sync.Once

func ListeningSessionService() *listeningSessionService {
	listeningSessionServiceOnce.Do(func() {
		listeningSessionServiceInstance = &listeningSessionService{
			numberRunes: []rune("0123456789"),
		}
	})
	return listeningSessionServiceInstance
}

func (listeningSessionService) GetTotalSessionCount() int {
	var count int
	database.GetConnection().Model(&ListeningSession{}).Count(&count)
	return count
}

func (listeningSessionService) GetActiveSessionCount() int {
	var count int
	database.GetConnection().Model(&ListeningSession{}).Where(ListeningSession{Active: true}).Count(&count)
	return count
}

func (listeningSessionService) GetActiveSessions() []ListeningSession {
	var sessions []ListeningSession
	database.GetConnection().Where(ListeningSession{Active: true}).Find(&sessions)
	return sessions
}

func (listeningSessionService) GetSessionById(id uint) *ListeningSession {
	var sessions []ListeningSession
	database.GetConnection().Where(ListeningSession{Model: gorm.Model{ID: id}}).Find(&sessions)

	if len(sessions) == 1 {
		return &sessions[0]
	} else {
		return nil
	}
}

func (listeningSessionService) GetSessionByJoinId(joinId string) *ListeningSession {
	if len(joinId) == 0 {
		return nil
	}

	var sessions []ListeningSession
	database.GetConnection().Where(ListeningSession{JoinId: &joinId}).Find(&sessions)

	if len(sessions) == 1 {
		return &sessions[0]
	} else {
		return nil
	}
}

func (listeningSessionService) GetActiveSessionsByOwnerId(ownerId uint) []ListeningSession {
	var sessions []ListeningSession
	database.GetConnection().Where(ListeningSession{Active: true, OwnerId: ownerId}).Find(&sessions)
	return sessions
}

func (s listeningSessionService) GetCurrentlyPlayingRequest(session ListeningSession) *SongRequest {
	var requests []SongRequest
	database.GetConnection().Where(SongRequest{
		SessionId: session.ID,
		Status:    StatusCurrentlyPlaying,
	}, session.ID).Find(&requests)

	if len(requests) > 0 {
		return &requests[0]
	} else {
		return nil
	}
}

func (s listeningSessionService) GetUpNextRequest(session ListeningSession) *SongRequest {
	var requests []SongRequest
	database.GetConnection().Where(SongRequest{
		SessionId: session.ID,
		Status:    StatusUpNext,
	}, session.ID).Find(&requests)

	if len(requests) > 0 {
		return &requests[0]
	} else {
		return nil
	}
}

func (s listeningSessionService) GetSessionQueueInDemocraticOrder(session ListeningSession) []SongRequest {
	var requests []SongRequest
	database.GetConnection().Where(SongRequest{
		SessionId: session.ID,
		Status:    StatusInQueue,
	}).Order("created_at asc").Find(&requests)

	// TODO: Do something smarter than just using the request order here

	return requests
}

func (s listeningSessionService) NewSession(user User, title string) (*ListeningSession, error) {
	if len(title) == 0 {
		return nil, errors.New("title must not be empty")
	}

	client := SpotifyService().GetClientForUser(user)

	joinId := s.newJoinId()
	playlist, err := client.CreatePlaylistForUser(user.SpotifyId, fmt.Sprintf("%s - SpotiFete", title), fmt.Sprintf("Automatic playlist for SpotiFete session %s. You can join using the code %s-%s or by installing our app and scanning the QR code in the playlist image.", title, joinId[0:4], joinId[4:8]), false)
	if err != nil {
		return nil, err
	}

	// Generate QR code for this session
	qrCode, err := s.GenerateQrCodeForSession(joinId, false)
	if err != nil {
		return nil, err
	}

	// Encode QR code as jpeg
	jpegBuffer := new(bytes.Buffer)
	err = jpeg.Encode(jpegBuffer, qrCode.Image(512), nil)
	if err != nil {
		return nil, err
	}

	// Set QR code as playlist image in background
	go func() {
		err := client.SetPlaylistImage(playlist.ID, jpegBuffer)
		if err != nil {
			logger.Error(err)
			sentry.CaptureException(err)
		}
	}()

	// Create database entry
	listeningSession := ListeningSession{
		Model:         gorm.Model{},
		Active:        true,
		OwnerId:       user.ID,
		JoinId:        &joinId,
		QueuePlaylist: playlist.ID.String(),
		Title:         title,
	}

	database.GetConnection().Create(&listeningSession)

	return &listeningSession, nil
}

func (s listeningSessionService) newJoinId() string {
	for {
		b := make([]rune, 8)
		for i := range b {
			b[i] = s.numberRunes[rand.Intn(len(s.numberRunes))]
		}
		newJoinId := string(b)

		if !s.joinIdExists(newJoinId) {
			return newJoinId
		}
	}
}

func (listeningSessionService) joinIdExists(joinId string) bool {
	var count uint
	database.GetConnection().Model(&ListeningSession{}).Where(ListeningSession{JoinId: &joinId}).Count(&count)
	return count > 0
}

func (s listeningSessionService) CloseSession(user User, joinId string) error {
	session := s.GetSessionByJoinId(joinId)
	if user.ID != session.OwnerId {
		return errors.New("only the owner can close a session")
	}

	session.Active = false
	session.JoinId = nil
	database.GetConnection().Save(&session)

	client := SpotifyService().GetClientForUser(user)
	err := client.UnfollowPlaylist(spotify.ID(user.SpotifyId), spotify.ID(session.QueuePlaylist))
	if err != nil {
		return err
	}

	// Create rewind playlist if any tracks were requested
	distinctRequestedTracks := s.GetDistinctRequestedTracks(*session)
	if len(distinctRequestedTracks) > 0 {
		rewindPlaylist, err := client.CreatePlaylistForUser(user.SpotifyId, fmt.Sprintf("%s Rewind - SpotiFete", session.Title), fmt.Sprintf("Rewind playlist for your session %s. This contains all the songs that were requested.", session.Title), false)
		if err != nil {
			return err
		}

		var page []spotify.ID
		for _, track := range distinctRequestedTracks {
			page = append(page, track)

			if len(page) == 100 {
				_, err = client.AddTracksToPlaylist(rewindPlaylist.ID, page...)
				if err != nil {
					return err
				}
				page = []spotify.ID{}
			}
		}

		if len(page) > 0 {
			_, err = client.AddTracksToPlaylist(rewindPlaylist.ID, page...)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (s listeningSessionService) IsTrackInQueue(session ListeningSession, trackId string) bool {
	var duplicateRequestsForTrack []SongRequest
	database.GetConnection().Where("status != 'PLAYED' AND session_id = ? AND spotify_track_id = ?", session.ID, trackId).Find(&duplicateRequestsForTrack)
	return len(duplicateRequestsForTrack) > 0
}

func (s listeningSessionService) RequestSong(session ListeningSession, trackId string) error {
	sessionOwner := UserService().GetUserById(session.OwnerId)
	client := SpotifyService().GetClientForUser(*sessionOwner)

	// Prevent duplicates
	if s.IsTrackInQueue(session, trackId) {
		return errors.New("that song is already in the queue")
	}

	// Check if we have to add the request to the queue or play it immediately
	currentlyPlayingRequest := s.GetCurrentlyPlayingRequest(session)
	upNextRequest := s.GetUpNextRequest(session)

	var newRequestStatus SongRequestStatus
	if currentlyPlayingRequest == nil {
		// No song is playing, that means the queue is empty -> Set this to play immediately
		newRequestStatus = StatusCurrentlyPlaying
	} else if upNextRequest == nil {
		// A song is currently playing, but no follow up song is present -> Set this as the next song
		newRequestStatus = StatusUpNext
	} else {
		// A song is currently playing and a follow up song is present. -> Just add this song to the normal queue
		newRequestStatus = StatusInQueue
	}

	updatedTrackMetadata, err := SpotifyService().AddOrUpdateTrackMetadata(*client, spotify.ID(trackId))
	if err != nil {
		return err
	}

	newSongRequest := SongRequest{
		Model:          gorm.Model{},
		SessionId:      session.ID,
		UserId:         nil,
		SpotifyTrackId: updatedTrackMetadata.SpotifyTrackId,
		Status:         newRequestStatus,
	}

	database.GetConnection().Create(&newSongRequest)

	return s.UpdateSessionPlaylistIfNeccessary(session)
}

func (s listeningSessionService) UpdateSessionIfNeccessary(session ListeningSession) error {
	currentlyPlayingRequest := s.GetCurrentlyPlayingRequest(session)
	upNextRequest := s.GetUpNextRequest(session)

	owner := UserService().GetUserById(session.OwnerId)
	client := SpotifyService().GetClientForUser(*owner)
	currentlyPlaying, err := client.PlayerCurrentlyPlaying()
	if err != nil {
		logger.Error(err)
		sentry.CaptureException(err)
		return err
	}

	if currentlyPlaying == nil || currentlyPlaying.Item == nil {
		// Nothing is running -> still update the playlist if neccessary
		return s.UpdateSessionPlaylistIfNeccessary(session)
	}

	currentlyPlayingSpotifyTrackId := currentlyPlaying.Item.ID.String()

	if currentlyPlayingRequest == nil {
		// No requests present
		// TODO: A this point we could use a fallback playlist or replay previously played tracks from this session
		return nil
	}

	if upNextRequest != nil && upNextRequest.SpotifyTrackId == currentlyPlayingSpotifyTrackId {
		// The previous track finished and the playlist moved on the the next track. Time to update!
		currentlyPlayingRequest.Status = StatusPlayed
		database.GetConnection().Save(currentlyPlayingRequest)

		upNextRequest.Status = StatusCurrentlyPlaying
		database.GetConnection().Save(upNextRequest)

		queue := s.GetSessionQueueInDemocraticOrder(session)
		if len(queue) > 0 {
			newUpNext := queue[0]
			newUpNext.Status = StatusUpNext
			database.GetConnection().Save(&newUpNext)
		}
	}

	return s.UpdateSessionPlaylistIfNeccessary(session)
}

func (s listeningSessionService) UpdateSessionPlaylistIfNeccessary(session ListeningSession) error {
	currentlyPlayingRequest := s.GetCurrentlyPlayingRequest(session)
	upNextRequest := s.GetUpNextRequest(session)

	if currentlyPlayingRequest == nil && upNextRequest == nil {
		return nil
	}

	owner := UserService().GetUserById(session.OwnerId)
	client := SpotifyService().GetClientForUser(*owner)

	playlist, err := client.GetPlaylist(spotify.ID(session.QueuePlaylist))
	if err != nil {
		return err
	}

	playlistTracks := playlist.Tracks.Tracks

	// First, check playlist length
	if currentlyPlayingRequest != nil && upNextRequest != nil && len(playlistTracks) != 2 {
		return s.updateSessionPlaylist(*client, session)
	}

	if currentlyPlayingRequest != nil && upNextRequest == nil && len(playlistTracks) != 1 {
		return s.updateSessionPlaylist(*client, session)
	}

	if currentlyPlayingRequest == nil && upNextRequest == nil && len(playlistTracks) != 0 {
		return s.updateSessionPlaylist(*client, session)
	}

	// Second, check playlist content
	if currentlyPlayingRequest != nil {
		if playlistTracks[0].Track.ID.String() != currentlyPlayingRequest.SpotifyTrackId {
			return s.updateSessionPlaylist(*client, session)
		}

		if upNextRequest != nil {
			if playlistTracks[1].Track.ID.String() != upNextRequest.SpotifyTrackId {
				return s.updateSessionPlaylist(*client, session)
			}
		}
	}

	return nil
}

func (s listeningSessionService) updateSessionPlaylist(client spotify.Client, session ListeningSession) error {
	currentlyPlayingRequest := s.GetCurrentlyPlayingRequest(session)
	upNextRequest := s.GetUpNextRequest(session)

	playlistId := spotify.ID(session.QueuePlaylist)

	// Always replace all tracks with only the current one playing first
	err := client.ReplacePlaylistTracks(playlistId, spotify.ID(currentlyPlayingRequest.SpotifyTrackId))
	if err != nil {
		logger.Error(err)
		sentry.CaptureException(err)
		return err
	}

	// After that, add the up next song as well if it is present
	if upNextRequest != nil {
		_, err = client.AddTracksToPlaylist(playlistId, spotify.ID(upNextRequest.SpotifyTrackId))
		if err != nil {
			logger.Error(err)
			sentry.CaptureException(err)
			return err
		}
	}

	return nil
}

func (s listeningSessionService) PollSessions() {
	for range time.Tick(5 * time.Second) {
		for _, session := range s.GetActiveSessions() {
			err := s.UpdateSessionIfNeccessary(session)
			if err != nil {
				logger.Errorf("error while polling session %s: %s", *session.JoinId, err.Error())
				sentry.CaptureException(err)
			}
		}
	}
}

func (s listeningSessionService) CreateDto(listeningSession ListeningSession, resolveAdditionalInformation bool) dto.ListeningSessionDto {
	result := dto.ListeningSessionDto{}
	if listeningSession.JoinId == nil {
		result.JoinId = ""
	} else {
		result.JoinId = *listeningSession.JoinId

	}
	result.JoinIdHumanReadable = fmt.Sprintf("%s %s", result.JoinId[0:4], result.JoinId[4:8])
	result.Title = listeningSession.Title

	if resolveAdditionalInformation {
		owner := UserService().GetUserById(listeningSession.OwnerId)
		result.Owner = UserService().CreateDto(*owner, false)
		result.QueuePlaylistId = listeningSession.QueuePlaylist

		currentlyPlayingRequest := s.GetCurrentlyPlayingRequest(listeningSession)
		upNextRequest := s.GetUpNextRequest(listeningSession)

		if currentlyPlayingRequest != nil {
			currentlyPlayingRequestTrack := dto.TrackMetadataDto{}.FromDatabaseModel(*SpotifyService().GetTrackMetadataBySpotifyTrackId(currentlyPlayingRequest.SpotifyTrackId))
			result.CurrentlyPlaying = &currentlyPlayingRequestTrack
		}

		if upNextRequest != nil {
			upNextRequestTrack := dto.TrackMetadataDto{}.FromDatabaseModel(*SpotifyService().GetTrackMetadataBySpotifyTrackId(upNextRequest.SpotifyTrackId))
			result.UpNext = &upNextRequestTrack
		}

		result.Queue = []dto.TrackMetadataDto{}
		for _, request := range s.GetSessionQueueInDemocraticOrder(listeningSession) {
			requestTrack := SpotifyService().GetTrackMetadataBySpotifyTrackId(request.SpotifyTrackId)
			result.Queue = append(result.Queue, dto.TrackMetadataDto{}.FromDatabaseModel(*requestTrack))
		}

		result.QueueLastUpdated = s.GetQueueLastUpdated(listeningSession)
	}

	return result
}

func (s listeningSessionService) GetQueueLastUpdated(session ListeningSession) time.Time {
	lastUpdatedSongRequest := SongRequest{}
	database.GetConnection().Where(SongRequest{
		SessionId: session.ID,
	}).Order("updated_at desc").First(&lastUpdatedSongRequest)

	if lastUpdatedSongRequest.ID != 0 {
		return lastUpdatedSongRequest.UpdatedAt
	} else {
		// No requests found -> Use creation of session
		return session.UpdatedAt
	}
}

func (s listeningSessionService) GetDistinctRequestedTracks(session ListeningSession) (trackIds []spotify.ID) {
	type Result struct {
		SpotifyTrackId string
	}

	var results []Result
	database.GetConnection().Table("song_requests").Select("distinct spotify_track_id").Where(SongRequest{
		SessionId: session.ID,
	}).Scan(&results)

	for _, result := range results {
		trackIds = append(trackIds, spotify.ID(result.SpotifyTrackId))
	}

	return
}

func (listeningSessionService) GenerateQrCodeForSession(joinId string, disableBorder bool) (*qrcode.QRCode, error) {
	// Generate QR code for this session
	qrCode, err := qrcode.New(fmt.Sprintf("spotifete://session/%s", joinId), qrcode.Medium)
	if err != nil {
		return nil, err
	}

	qrCode.DisableBorder = disableBorder
	return qrCode, nil
}
