package library

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var (
	// ErrRenameTemplatesInvalid is returned when required rename templates are empty.
	ErrRenameTemplatesInvalid = errors.New("library: rename templates are invalid")
	// ErrRenameUnsupportedEntity indicates the targeted entity does not support renaming.
	ErrRenameUnsupportedEntity = errors.New("library: rename not supported for entity type")
	// ErrRenameNoFiles signals that there are no files registered for an entity.
	ErrRenameNoFiles = errors.New("library: no files available for rename job")
)

// RenameOptions controls execution behavior for rename jobs.
type RenameOptions struct {
	DryRun bool
	Force  bool
}

// RenamePreview summarizes the operations that would be executed by a rename job.
type RenamePreview struct {
	EntityID        string
	EntityType      string
	EpisodeTemplate *EpisodeTemplateSet
	MovieTemplate   *MovieTemplateSet
	MusicTemplate   *MusicTemplateSet
	Operations      []RenameOperation
	Changes         int
}

// RenameJobRecord captures a persisted rename job execution.
type RenameJobRecord struct {
	ID              string
	EntityID        string
	EntityType      string
	RequestedAt     time.Time
	DryRun          bool
	EpisodeTemplate *EpisodeTemplateSet
	MovieTemplate   *MovieTemplateSet
	MusicTemplate   *MusicTemplateSet
	Operations      []RenameOperation
	Changes         int
}

// RenameOperation models the outcome of a single file rename decision.
type RenameOperation struct {
	FileID           string
	WorkType         string
	SourcePath       string
	TargetPath       string
	ShouldRename     bool
	Reason           string
	LastRenamedAt    time.Time
	LastRenamedJobID string
}

// LibraryFile tracks imported media files and their metadata for rename planning.
type LibraryFile struct {
	ID             string
	EntityID       string
	WorkType       string
	RelativePath   string
	Extension      string
	Quality        string
	Resolution     string
	Edition        string
	Source         string
	SeriesTitle    string
	SeasonNumber   int
	EpisodeNumber  int
	AbsoluteNumber int
	EpisodeTitle   string
	ReleaseYear    int
	MovieTitle     string
	ArtistName     string
	AlbumTitle     string
	TrackTitle     string
	TrackNumber    int
	DiscNumber     int
}

// EpisodeTemplateSet defines folder and file formatting for episodic media.
type EpisodeTemplateSet struct {
	SeriesFolderPattern string
	SeasonFolderPattern string
	EpisodeFilePattern  string
}

// MovieTemplateSet defines folder and file formatting for movies.
type MovieTemplateSet struct {
	MovieFolderPattern string
	MovieFilePattern   string
}

// MusicTemplateSet defines folder and file formatting for music libraries.
type MusicTemplateSet struct {
	ArtistFolderPattern string
	AlbumFolderPattern  string
	TrackFilePattern    string
}

// RenameTemplates aggregates all template sets.
type RenameTemplates struct {
	Episode EpisodeTemplateSet
	Movie   MovieTemplateSet
	Music   MusicTemplateSet
}

func (t RenameTemplates) WithDefaults() RenameTemplates {
	defaults := DefaultRenameTemplates()

	if strings.TrimSpace(t.Episode.SeriesFolderPattern) == "" {
		t.Episode.SeriesFolderPattern = defaults.Episode.SeriesFolderPattern
	}
	if strings.TrimSpace(t.Episode.SeasonFolderPattern) == "" {
		t.Episode.SeasonFolderPattern = defaults.Episode.SeasonFolderPattern
	}
	if strings.TrimSpace(t.Episode.EpisodeFilePattern) == "" {
		t.Episode.EpisodeFilePattern = defaults.Episode.EpisodeFilePattern
	}

	if strings.TrimSpace(t.Movie.MovieFolderPattern) == "" {
		t.Movie.MovieFolderPattern = defaults.Movie.MovieFolderPattern
	}
	if strings.TrimSpace(t.Movie.MovieFilePattern) == "" {
		t.Movie.MovieFilePattern = defaults.Movie.MovieFilePattern
	}

	if strings.TrimSpace(t.Music.ArtistFolderPattern) == "" {
		t.Music.ArtistFolderPattern = defaults.Music.ArtistFolderPattern
	}
	if strings.TrimSpace(t.Music.AlbumFolderPattern) == "" {
		t.Music.AlbumFolderPattern = defaults.Music.AlbumFolderPattern
	}
	if strings.TrimSpace(t.Music.TrackFilePattern) == "" {
		t.Music.TrackFilePattern = defaults.Music.TrackFilePattern
	}

	return t
}

// Validate ensures the templates contain required patterns.
func (t RenameTemplates) Validate() error {
	trimmed := t.WithDefaults()

	switch {
	case strings.TrimSpace(trimmed.Episode.SeriesFolderPattern) == "":
		return ErrRenameTemplatesInvalid
	case strings.TrimSpace(trimmed.Episode.SeasonFolderPattern) == "":
		return ErrRenameTemplatesInvalid
	case strings.TrimSpace(trimmed.Episode.EpisodeFilePattern) == "":
		return ErrRenameTemplatesInvalid
	case strings.TrimSpace(trimmed.Movie.MovieFolderPattern) == "":
		return ErrRenameTemplatesInvalid
	case strings.TrimSpace(trimmed.Movie.MovieFilePattern) == "":
		return ErrRenameTemplatesInvalid
	case strings.TrimSpace(trimmed.Music.ArtistFolderPattern) == "":
		return ErrRenameTemplatesInvalid
	case strings.TrimSpace(trimmed.Music.AlbumFolderPattern) == "":
		return ErrRenameTemplatesInvalid
	case strings.TrimSpace(trimmed.Music.TrackFilePattern) == "":
		return ErrRenameTemplatesInvalid
	default:
		return nil
	}
}

// DefaultRenameTemplates returns sensible defaults inspired by the *arr ecosystem.
func DefaultRenameTemplates() RenameTemplates {
	return RenameTemplates{
		Episode: EpisodeTemplateSet{
			SeriesFolderPattern: "{SeriesTitle}",
			SeasonFolderPattern: "Season {SeasonNumber:00}",
			EpisodeFilePattern:  "{SeriesTitle} - S{SeasonNumber:00}E{EpisodeNumber:00} - {EpisodeTitle} [{Quality}].{Extension:lower}",
		},
		Movie: MovieTemplateSet{
			MovieFolderPattern: "{MovieTitle} ({ReleaseYear})",
			MovieFilePattern:   "{MovieTitle} ({ReleaseYear}) [{Quality}].{Extension:lower}",
		},
		Music: MusicTemplateSet{
			ArtistFolderPattern: "{ArtistName}",
			AlbumFolderPattern:  "{AlbumTitle} ({ReleaseYear})",
			TrackFilePattern:    "{TrackNumber:00} - {TrackTitle}.{Extension:lower}",
		},
	}
}

// ConfigureRenameTemplates updates the global rename templates.
func (s *Service) ConfigureRenameTemplates(ctx context.Context, templates RenameTemplates) (RenameTemplates, error) {
	normalized := templates.WithDefaults()
	if err := normalized.Validate(); err != nil {
		return RenameTemplates{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.renameTemplates = normalized
	return s.renameTemplates, nil
}

// ListRenameTemplates returns the currently configured rename templates.
func (s *Service) ListRenameTemplates(ctx context.Context) RenameTemplates {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.renameTemplates
}

// PreviewRename builds a rename plan without mutating state.
func (s *Service) PreviewRename(ctx context.Context, entityID string) (RenamePreview, error) {
	s.mu.RLock()
	entity, ok := s.entitiesByID[strings.TrimSpace(entityID)]
	if !ok {
		s.mu.RUnlock()
		return RenamePreview{}, ErrUnknownEntity
	}
	files := cloneLibraryFiles(s.libraryFiles[entity.ID])
	templates := s.renameTemplates
	state := cloneRenameState(s.renameState[entity.ID])
	s.mu.RUnlock()

	if len(files) == 0 {
		return RenamePreview{}, ErrRenameNoFiles
	}

	plan, err := buildRenamePlan(entity, files, templates, state)
	if err != nil {
		return RenamePreview{}, err
	}

	return RenamePreview{
		EntityID:        entity.ID,
		EntityType:      entity.EntityType,
		EpisodeTemplate: plan.episodeTemplate,
		MovieTemplate:   plan.movieTemplate,
		MusicTemplate:   plan.musicTemplate,
		Operations:      plan.operations,
		Changes:         plan.changes,
	}, nil
}

// StartRenameJob executes a rename plan and records history when not running in dry-run mode.
func (s *Service) StartRenameJob(ctx context.Context, entityID string, opts RenameOptions) (RenameJobRecord, error) {
	trimmed := strings.TrimSpace(entityID)

	s.mu.Lock()
	defer s.mu.Unlock()

	entity, ok := s.entitiesByID[trimmed]
	if !ok {
		return RenameJobRecord{}, ErrUnknownEntity
	}

	files := cloneLibraryFiles(s.libraryFiles[entity.ID])
	if len(files) == 0 {
		return RenameJobRecord{}, ErrRenameNoFiles
	}

	plan, err := buildRenamePlan(entity, files, s.renameTemplates, s.renameState[entity.ID])
	if err != nil {
		return RenameJobRecord{}, err
	}

	jobID := s.idGen()
	now := s.clock.Now()
	record := RenameJobRecord{
		ID:              jobID,
		EntityID:        entity.ID,
		EntityType:      entity.EntityType,
		RequestedAt:     now,
		DryRun:          opts.DryRun,
		EpisodeTemplate: plan.episodeTemplate,
		MovieTemplate:   plan.movieTemplate,
		MusicTemplate:   plan.musicTemplate,
		Operations:      cloneRenameOperations(plan.operations),
		Changes:         plan.changes,
	}

	if opts.DryRun {
		return record, nil
	}

	if _, ok := s.renameState[entity.ID]; !ok {
		s.renameState[entity.ID] = make(map[string]renameState)
	}

	for idx := range record.Operations {
		op := &record.Operations[idx]
		if strings.TrimSpace(op.TargetPath) == "" {
			continue
		}
		if op.ShouldRename || opts.Force {
			if !op.ShouldRename && opts.Force {
				if strings.TrimSpace(op.Reason) == "" {
					op.Reason = "rename forced by operator"
				} else {
					op.Reason = fmt.Sprintf("%s (forced)", op.Reason)
				}
			}
			applyRenameToLibrary(s.libraryFiles, s.libraryFileIdx, entity.ID, op.FileID, op.TargetPath)
			op.ShouldRename = true
			op.LastRenamedAt = now
			op.LastRenamedJobID = record.ID
			s.renameState[entity.ID][op.FileID] = renameState{LastPath: op.TargetPath, JobID: record.ID, CompletedAt: now}
		}
	}

	s.renameHistory[entity.ID] = append(s.renameHistory[entity.ID], record)
	return record, nil
}

// RenameHistory returns the recorded rename jobs for an entity ordered from newest to oldest.
func (s *Service) RenameHistory(ctx context.Context, entityID string, limit int) ([]RenameJobRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entity, ok := s.entitiesByID[strings.TrimSpace(entityID)]
	if !ok {
		return nil, ErrUnknownEntity
	}

	history := s.renameHistory[entity.ID]
	if len(history) == 0 {
		return nil, nil
	}

	cloned := make([]RenameJobRecord, len(history))
	copy(cloned, history)
	sort.Slice(cloned, func(i, j int) bool {
		return cloned[i].RequestedAt.After(cloned[j].RequestedAt)
	})
	if limit > 0 && len(cloned) > limit {
		cloned = cloned[:limit]
	}
	for i := range cloned {
		cloned[i].Operations = cloneRenameOperations(cloned[i].Operations)
	}
	return cloned, nil
}

// SetLibraryFiles replaces the tracked files for an entity. Primarily used in tests.
func (s *Service) SetLibraryFiles(entityID string, files []LibraryFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	trimmed := strings.TrimSpace(entityID)
	entity, ok := s.entitiesByID[trimmed]
	if !ok {
		return ErrUnknownEntity
	}

	copies := make([]LibraryFile, len(files))
	copy(copies, files)
	s.libraryFiles[entity.ID] = copies
	index := make(map[string]int, len(copies))
	for i := range copies {
		index[copies[i].ID] = i
	}
	s.libraryFileIdx[entity.ID] = index
	return nil
}

type renamePlan struct {
	operations      []RenameOperation
	changes         int
	episodeTemplate *EpisodeTemplateSet
	movieTemplate   *MovieTemplateSet
	musicTemplate   *MusicTemplateSet
}

type renameState struct {
	LastPath    string
	JobID       string
	CompletedAt time.Time
}

func buildRenamePlan(entity Entity, files []LibraryFile, templates RenameTemplates, state map[string]renameState) (renamePlan, error) {
	switch entity.EntityType {
	case "series":
		return planForSeries(entity, files, templates.Episode, state)
	case "movie":
		return planForMovies(entity, files, templates.Movie, state)
	case "artist", "album":
		return planForMusic(entity, files, templates.Music, state)
	default:
		return renamePlan{}, ErrRenameUnsupportedEntity
	}
}

func planForSeries(entity Entity, files []LibraryFile, templates EpisodeTemplateSet, state map[string]renameState) (renamePlan, error) {
	operations := make([]RenameOperation, 0, len(files))
	metadataBase := map[string]string{
		"SeriesTitle": entity.Title,
		"EntityTitle": entity.Title,
	}

	for _, file := range files {
		if file.WorkType != "episode" {
			continue
		}
		metadata := buildEpisodeMetadata(metadataBase, file)
		targetPath := joinPath(
			applyPattern(templates.SeriesFolderPattern, metadata),
			applyPattern(templates.SeasonFolderPattern, metadata),
			applyPattern(templates.EpisodeFilePattern, metadata),
		)
		op := buildOperation(file, targetPath, state[file.ID])
		operations = append(operations, op)
	}

	if len(operations) == 0 {
		return renamePlan{}, ErrRenameNoFiles
	}

	tmpl := templates
	return finalizePlan(operations, &tmpl, nil, nil), nil
}

func planForMovies(entity Entity, files []LibraryFile, templates MovieTemplateSet, state map[string]renameState) (renamePlan, error) {
	operations := make([]RenameOperation, 0, len(files))
	metadataBase := map[string]string{
		"MovieTitle":  entity.Title,
		"EntityTitle": entity.Title,
	}

	for _, file := range files {
		if file.WorkType != "movie" {
			continue
		}
		metadata := buildMovieMetadata(metadataBase, file)
		targetPath := joinPath(
			applyPattern(templates.MovieFolderPattern, metadata),
			applyPattern(templates.MovieFilePattern, metadata),
		)
		op := buildOperation(file, targetPath, state[file.ID])
		operations = append(operations, op)
	}

	if len(operations) == 0 {
		return renamePlan{}, ErrRenameNoFiles
	}

	tmpl := templates
	return finalizePlan(operations, nil, &tmpl, nil), nil
}

func planForMusic(entity Entity, files []LibraryFile, templates MusicTemplateSet, state map[string]renameState) (renamePlan, error) {
	operations := make([]RenameOperation, 0, len(files))
	metadataBase := map[string]string{
		"ArtistName":  entity.Title,
		"EntityTitle": entity.Title,
	}

	for _, file := range files {
		if file.WorkType != "track" {
			continue
		}
		metadata := buildMusicMetadata(metadataBase, file)
		targetPath := joinPath(
			applyPattern(templates.ArtistFolderPattern, metadata),
			applyPattern(templates.AlbumFolderPattern, metadata),
			applyPattern(templates.TrackFilePattern, metadata),
		)
		op := buildOperation(file, targetPath, state[file.ID])
		operations = append(operations, op)
	}

	if len(operations) == 0 {
		return renamePlan{}, ErrRenameNoFiles
	}

	tmpl := templates
	return finalizePlan(operations, nil, nil, &tmpl), nil
}

func finalizePlan(operations []RenameOperation, episode *EpisodeTemplateSet, movie *MovieTemplateSet, music *MusicTemplateSet) renamePlan {
	// Detect conflicting target paths.
	conflicts := make(map[string][]int)
	for idx, op := range operations {
		if op.TargetPath == "" {
			continue
		}
		conflicts[op.TargetPath] = append(conflicts[op.TargetPath], idx)
	}

	for _, indexes := range conflicts {
		if len(indexes) <= 1 {
			continue
		}
		ids := make([]string, 0, len(indexes))
		for _, idx := range indexes {
			ids = append(ids, operations[idx].FileID)
		}
		reason := fmt.Sprintf("target path conflict with files %s", strings.Join(ids, ", "))
		for _, idx := range indexes {
			operations[idx].ShouldRename = false
			operations[idx].Reason = reason
		}
	}

	changes := 0
	for idx := range operations {
		if operations[idx].ShouldRename {
			changes++
		}
	}

	return renamePlan{
		operations:      operations,
		changes:         changes,
		episodeTemplate: episode,
		movieTemplate:   movie,
		musicTemplate:   music,
	}
}

func buildOperation(file LibraryFile, targetPath string, state renameState) RenameOperation {
	op := RenameOperation{
		FileID:     file.ID,
		WorkType:   file.WorkType,
		SourcePath: strings.TrimPrefix(strings.TrimSpace(file.RelativePath), string(filepath.Separator)),
		TargetPath: targetPath,
	}

	if state.JobID != "" {
		op.LastRenamedJobID = state.JobID
		op.LastRenamedAt = state.CompletedAt
	}

	cleanSource := strings.TrimSpace(op.SourcePath)
	cleanTarget := strings.TrimSpace(targetPath)

	switch {
	case cleanTarget == "":
		op.ShouldRename = false
		op.Reason = "template produced empty target"
	case cleanSource == cleanTarget:
		op.ShouldRename = false
		if state.JobID != "" {
			op.Reason = fmt.Sprintf("already aligned via job %s", state.JobID)
		} else {
			op.Reason = "already matches target path"
		}
	default:
		op.ShouldRename = true
		if state.LastPath != "" && state.LastPath != cleanTarget {
			op.Reason = fmt.Sprintf("updated from previous target %s", state.LastPath)
		} else if state.LastPath == cleanTarget && state.JobID != "" {
			op.Reason = fmt.Sprintf("restoring path from job %s", state.JobID)
		}
	}

	return op
}

func buildEpisodeMetadata(base map[string]string, file LibraryFile) map[string]string {
	meta := cloneStringMap(base)
	setIfNotEmpty(meta, "SeriesTitle", file.SeriesTitle)
	setIfNotEmpty(meta, "SeasonNumber", strconv.Itoa(file.SeasonNumber))
	setIfNotEmpty(meta, "EpisodeNumber", strconv.Itoa(file.EpisodeNumber))
	setIfNotEmpty(meta, "AbsoluteEpisodeNumber", strconv.Itoa(file.AbsoluteNumber))
	setIfNotEmpty(meta, "EpisodeTitle", file.EpisodeTitle)
	setIfPositive(meta, "ReleaseYear", file.ReleaseYear)
	setIfNotEmpty(meta, "Quality", file.Quality)
	setIfNotEmpty(meta, "Resolution", file.Resolution)
	setIfNotEmpty(meta, "Edition", file.Edition)
	setIfNotEmpty(meta, "Source", file.Source)
	setIfNotEmpty(meta, "Extension", normalizeExtension(file.Extension, file.RelativePath))
	return meta
}

func buildMovieMetadata(base map[string]string, file LibraryFile) map[string]string {
	meta := cloneStringMap(base)
	setIfNotEmpty(meta, "MovieTitle", file.MovieTitle)
	setIfPositive(meta, "ReleaseYear", file.ReleaseYear)
	setIfNotEmpty(meta, "Quality", file.Quality)
	setIfNotEmpty(meta, "Resolution", file.Resolution)
	setIfNotEmpty(meta, "Edition", file.Edition)
	setIfNotEmpty(meta, "Source", file.Source)
	setIfNotEmpty(meta, "Extension", normalizeExtension(file.Extension, file.RelativePath))
	return meta
}

func buildMusicMetadata(base map[string]string, file LibraryFile) map[string]string {
	meta := cloneStringMap(base)
	setIfNotEmpty(meta, "ArtistName", file.ArtistName)
	setIfNotEmpty(meta, "AlbumTitle", file.AlbumTitle)
	setIfNotEmpty(meta, "TrackTitle", file.TrackTitle)
	setIfNotEmpty(meta, "TrackNumber", strconv.Itoa(file.TrackNumber))
	setIfNotEmpty(meta, "DiscNumber", strconv.Itoa(file.DiscNumber))
	setIfPositive(meta, "ReleaseYear", file.ReleaseYear)
	setIfNotEmpty(meta, "Quality", file.Quality)
	setIfNotEmpty(meta, "Resolution", file.Resolution)
	setIfNotEmpty(meta, "Extension", normalizeExtension(file.Extension, file.RelativePath))
	return meta
}

func normalizeExtension(ext string, path string) string {
	trimmed := strings.TrimPrefix(strings.TrimSpace(ext), ".")
	if trimmed != "" {
		return trimmed
	}
	base := filepath.Base(path)
	if dot := strings.LastIndex(base, "."); dot != -1 && dot < len(base)-1 {
		return base[dot+1:]
	}
	return ""
}

var tokenRegexp = regexp.MustCompile(`\{([A-Za-z0-9]+)(?::([^}]+))?\}`)

func applyPattern(pattern string, metadata map[string]string) string {
	if strings.TrimSpace(pattern) == "" {
		return ""
	}
	sanitized := tokenRegexp.ReplaceAllStringFunc(pattern, func(match string) string {
		groups := tokenRegexp.FindStringSubmatch(match)
		if len(groups) < 2 {
			return ""
		}
		key := groups[1]
		format := ""
		if len(groups) > 2 {
			format = groups[2]
		}
		value := sanitizePathValue(metadata[key])
		if value == "" {
			return ""
		}
		formatted := formatTokenValue(value, format)
		return finalizeComponent(formatted)
	})
	return finalizeComponent(sanitized)
}

func formatTokenValue(value, format string) string {
	if format == "" {
		return value
	}

	switch {
	case strings.HasPrefix(format, "0"):
		width := len(format)
		if n, err := strconv.Atoi(value); err == nil {
			return fmt.Sprintf("%0*d", width, n)
		}
	case format == "upper":
		return strings.ToUpper(value)
	case format == "lower":
		return strings.ToLower(value)
	case format == "title":
		return cases.Title(language.English).String(value)
	}

	return value
}

var forbiddenPathChars = strings.NewReplacer(
	"/", " ",
	"\\", " ",
	":", " - ",
	"*", "",
	"?", "",
	"\"", "",
	"<", "",
	">", "",
	"|", "",
	"\n", " ",
	"\r", " ",
	"\t", " ",
)

func sanitizePathValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	cleaned := forbiddenPathChars.Replace(trimmed)
	fields := strings.Fields(cleaned)
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}

var emptyParens = regexp.MustCompile(`\([\s\-_.]*\)`)
var repeatedSpaces = regexp.MustCompile(`\s{2,}`)

func finalizeComponent(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	v = emptyParens.ReplaceAllString(v, "")
	v = repeatedSpaces.ReplaceAllString(v, " ")
	v = strings.TrimSpace(v)
	v = strings.Trim(v, "-_. ")
	return v
}

func joinPath(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		component := finalizeComponent(part)
		if component != "" {
			cleaned = append(cleaned, component)
		}
	}
	return strings.Join(cleaned, "/")
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneLibraryFiles(files []LibraryFile) []LibraryFile {
	out := make([]LibraryFile, len(files))
	copy(out, files)
	return out
}

func cloneRenameOperations(ops []RenameOperation) []RenameOperation {
	out := make([]RenameOperation, len(ops))
	copy(out, ops)
	return out
}

func cloneRenameState(in map[string]renameState) map[string]renameState {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]renameState, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func setIfNotEmpty(target map[string]string, key, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		target[key] = value
	}
}

func setIfPositive(target map[string]string, key string, value int) {
	if value > 0 {
		target[key] = strconv.Itoa(value)
	}
}

func applyRenameToLibrary(library map[string][]LibraryFile, index map[string]map[string]int, entityID, fileID, newPath string) {
	files := library[entityID]
	entityIndex, ok := index[entityID]
	if !ok {
		return
	}
	idx, ok := entityIndex[fileID]
	if !ok {
		return
	}
	files[idx].RelativePath = newPath
	library[entityID] = files
}
