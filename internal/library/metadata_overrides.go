package library

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
)

// Metadata override validation errors.
var (
	ErrMetadataOverrideEntityRequired  = errors.New("library: metadata override entity id is required")
	ErrMetadataOverrideInvalidEpisode  = errors.New("library: metadata override episode id is required")
	ErrMetadataOverrideInvalidEdition  = errors.New("library: metadata override edition id is required")
	ErrMetadataOverrideInvalidTrack    = errors.New("library: metadata override track id is required")
	ErrMetadataOverrideInvalidTrackNum = errors.New("library: metadata override track number must be positive")
)

// EpisodeCutoverOverride captures season/episode renumbering for edge cases like split seasons
// or regional broadcast differences.
type EpisodeCutoverOverride struct {
	EpisodeID      string     `json:"episode_id"`
	SeasonNumber   int        `json:"season_number"`
	EpisodeNumber  int        `json:"episode_number"`
	AbsoluteNumber int        `json:"absolute_number,omitempty"`
	Title          string     `json:"title,omitempty"`
	AirDate        *time.Time `json:"air_date,omitempty"`
	Notes          string     `json:"notes,omitempty"`
}

// AlbumEditionOverride records alternate album packaging or bonus release editions so
// operators can align catalogs with preferred variants.
type AlbumEditionOverride struct {
	EditionID   string     `json:"edition_id"`
	Name        string     `json:"name"`
	ReleaseDate *time.Time `json:"release_date,omitempty"`
	IsDefault   bool       `json:"is_default,omitempty"`
	Notes       string     `json:"notes,omitempty"`
}

// TrackNumberOverride rewrites track ordering, disc assignments, or numbering gaps.
type TrackNumberOverride struct {
	TrackID     string `json:"track_id"`
	DiscNumber  int    `json:"disc_number,omitempty"`
	TrackNumber int    `json:"track_number"`
	TotalTracks int    `json:"total_tracks,omitempty"`
	ISRC        string `json:"isrc,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

// MetadataOverrides aggregates the override payloads for a specific entity.
type MetadataOverrides struct {
	EpisodeCutovers      []EpisodeCutoverOverride `json:"episode_cutovers,omitempty"`
	AlbumEditions        []AlbumEditionOverride   `json:"album_editions,omitempty"`
	TrackNumberOverrides []TrackNumberOverride    `json:"track_number_overrides,omitempty"`
	UpdatedAt            time.Time                `json:"updated_at"`
}

// GetMetadataOverrides retrieves override data for the provided entity.
func (s *Service) GetMetadataOverrides(ctx context.Context, entityID string) (overrides MetadataOverrides, found bool, err error) {
	ctx, span := s.startSpan(ctx, "library.Service.GetMetadataOverrides",
		attribute.String("library.entity_id", strings.TrimSpace(entityID)),
	)
	defer endSpan(span, &err)

	trimmedID := strings.TrimSpace(entityID)
	if trimmedID == "" {
		err = ErrMetadataOverrideEntityRequired
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.entityExistsLocked(trimmedID) {
		err = ErrUnknownEntity
		return
	}

	existing, ok := s.metadataOverrides[trimmedID]
	if !ok {
		return MetadataOverrides{}, false, nil
	}

	return cloneMetadataOverrides(existing), true, nil
}

// SetMetadataOverrides replaces the override configuration for an entity.
func (s *Service) SetMetadataOverrides(ctx context.Context, entityID string, payload MetadataOverrides) (overrides MetadataOverrides, err error) {
	ctx, span := s.startSpan(ctx, "library.Service.SetMetadataOverrides",
		attribute.String("library.entity_id", strings.TrimSpace(entityID)),
	)
	defer endSpan(span, &err)

	trimmedID := strings.TrimSpace(entityID)
	if trimmedID == "" {
		err = ErrMetadataOverrideEntityRequired
		return
	}

	normalized, err := normalizeMetadataOverrides(payload)
	if err != nil {
		return MetadataOverrides{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.entityExistsLocked(trimmedID) {
		err = ErrUnknownEntity
		return
	}

	normalized.UpdatedAt = s.clock.Now().UTC()
	if repoErr := s.repository.UpsertMetadataOverrides(ctx, trimmedID, normalized); repoErr != nil {
		err = fmt.Errorf("library: persist metadata overrides: %w", repoErr)
		return
	}
	s.metadataOverrides[trimmedID] = normalized

	span.SetAttributes(
		attribute.Int("library.metadata_override_episode_cutovers", len(normalized.EpisodeCutovers)),
		attribute.Int("library.metadata_override_album_editions", len(normalized.AlbumEditions)),
		attribute.Int("library.metadata_override_track_overrides", len(normalized.TrackNumberOverrides)),
	)
	s.logger.Info("library metadata overrides updated",
		"entityID", trimmedID,
		"episodeCutovers", len(normalized.EpisodeCutovers),
		"albumEditions", len(normalized.AlbumEditions),
		"trackOverrides", len(normalized.TrackNumberOverrides),
	)

	return cloneMetadataOverrides(normalized), nil
}

// ClearMetadataOverrides removes any stored overrides for an entity.
func (s *Service) ClearMetadataOverrides(ctx context.Context, entityID string) (err error) {
	ctx, span := s.startSpan(ctx, "library.Service.ClearMetadataOverrides",
		attribute.String("library.entity_id", strings.TrimSpace(entityID)),
	)
	defer endSpan(span, &err)

	trimmedID := strings.TrimSpace(entityID)
	if trimmedID == "" {
		err = ErrMetadataOverrideEntityRequired
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.entityExistsLocked(trimmedID) {
		err = ErrUnknownEntity
		return
	}

	if repoErr := s.repository.DeleteMetadataOverrides(ctx, trimmedID); repoErr != nil {
		err = fmt.Errorf("library: delete metadata overrides: %w", repoErr)
		return
	}

	delete(s.metadataOverrides, trimmedID)
	span.SetAttributes(attribute.Bool("library.metadata_override_deleted", true))
	s.logger.Info("library metadata overrides cleared", "entityID", trimmedID)
	return nil
}

func normalizeMetadataOverrides(input MetadataOverrides) (MetadataOverrides, error) {
	normalized := MetadataOverrides{}

	for _, cutover := range input.EpisodeCutovers {
		episodeID := strings.TrimSpace(cutover.EpisodeID)
		if episodeID == "" {
			return MetadataOverrides{}, ErrMetadataOverrideInvalidEpisode
		}
		normalized.EpisodeCutovers = append(normalized.EpisodeCutovers, EpisodeCutoverOverride{
			EpisodeID:      episodeID,
			SeasonNumber:   cutover.SeasonNumber,
			EpisodeNumber:  cutover.EpisodeNumber,
			AbsoluteNumber: cutover.AbsoluteNumber,
			Title:          strings.TrimSpace(cutover.Title),
			AirDate:        cloneTimePtr(cutover.AirDate),
			Notes:          strings.TrimSpace(cutover.Notes),
		})
	}
	sortEpisodeCutovers(normalized.EpisodeCutovers)

	for _, edition := range input.AlbumEditions {
		editionID := strings.TrimSpace(edition.EditionID)
		if editionID == "" {
			return MetadataOverrides{}, ErrMetadataOverrideInvalidEdition
		}
		normalized.AlbumEditions = append(normalized.AlbumEditions, AlbumEditionOverride{
			EditionID:   editionID,
			Name:        strings.TrimSpace(edition.Name),
			ReleaseDate: cloneTimePtr(edition.ReleaseDate),
			IsDefault:   edition.IsDefault,
			Notes:       strings.TrimSpace(edition.Notes),
		})
	}
	sortAlbumEditions(normalized.AlbumEditions)

	for _, track := range input.TrackNumberOverrides {
		trackID := strings.TrimSpace(track.TrackID)
		if trackID == "" {
			return MetadataOverrides{}, ErrMetadataOverrideInvalidTrack
		}
		if track.TrackNumber <= 0 {
			return MetadataOverrides{}, ErrMetadataOverrideInvalidTrackNum
		}
		normalized.TrackNumberOverrides = append(normalized.TrackNumberOverrides, TrackNumberOverride{
			TrackID:     trackID,
			DiscNumber:  track.DiscNumber,
			TrackNumber: track.TrackNumber,
			TotalTracks: track.TotalTracks,
			ISRC:        strings.TrimSpace(track.ISRC),
			Notes:       strings.TrimSpace(track.Notes),
		})
	}
	sortTrackNumberOverrides(normalized.TrackNumberOverrides)

	return normalized, nil
}

func cloneMetadataOverrides(src MetadataOverrides) MetadataOverrides {
	return MetadataOverrides{
		EpisodeCutovers:      cloneEpisodeCutovers(src.EpisodeCutovers),
		AlbumEditions:        cloneAlbumEditions(src.AlbumEditions),
		TrackNumberOverrides: cloneTrackOverrides(src.TrackNumberOverrides),
		UpdatedAt:            src.UpdatedAt,
	}
}

func cloneEpisodeCutovers(src []EpisodeCutoverOverride) []EpisodeCutoverOverride {
	if len(src) == 0 {
		return nil
	}
	out := make([]EpisodeCutoverOverride, len(src))
	for i := range src {
		out[i] = src[i]
		if src[i].AirDate != nil {
			copied := *src[i].AirDate
			out[i].AirDate = &copied
		}
	}
	return out
}

func cloneAlbumEditions(src []AlbumEditionOverride) []AlbumEditionOverride {
	if len(src) == 0 {
		return nil
	}
	out := make([]AlbumEditionOverride, len(src))
	for i := range src {
		out[i] = src[i]
		if src[i].ReleaseDate != nil {
			copied := *src[i].ReleaseDate
			out[i].ReleaseDate = &copied
		}
	}
	return out
}

func cloneTrackOverrides(src []TrackNumberOverride) []TrackNumberOverride {
	if len(src) == 0 {
		return nil
	}
	out := make([]TrackNumberOverride, len(src))
	copy(out, src)
	return out
}

func sortEpisodeCutovers(values []EpisodeCutoverOverride) {
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].SeasonNumber != values[j].SeasonNumber {
			return values[i].SeasonNumber < values[j].SeasonNumber
		}
		if values[i].EpisodeNumber != values[j].EpisodeNumber {
			return values[i].EpisodeNumber < values[j].EpisodeNumber
		}
		if values[i].AbsoluteNumber != values[j].AbsoluteNumber {
			return values[i].AbsoluteNumber < values[j].AbsoluteNumber
		}
		return strings.ToLower(values[i].EpisodeID) < strings.ToLower(values[j].EpisodeID)
	})
}

func sortAlbumEditions(values []AlbumEditionOverride) {
	sort.SliceStable(values, func(i, j int) bool {
		nameI := strings.ToLower(values[i].Name)
		nameJ := strings.ToLower(values[j].Name)
		if nameI != nameJ {
			return nameI < nameJ
		}
		return strings.ToLower(values[i].EditionID) < strings.ToLower(values[j].EditionID)
	})
}

func sortTrackNumberOverrides(values []TrackNumberOverride) {
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].DiscNumber != values[j].DiscNumber {
			return values[i].DiscNumber < values[j].DiscNumber
		}
		if values[i].TrackNumber != values[j].TrackNumber {
			return values[i].TrackNumber < values[j].TrackNumber
		}
		return strings.ToLower(values[i].TrackID) < strings.ToLower(values[j].TrackID)
	})
}

func cloneTimePtr(src *time.Time) *time.Time {
	if src == nil {
		return nil
	}
	copied := *src
	return &copied
}
