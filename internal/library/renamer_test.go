package library

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPreviewRenameSeries(t *testing.T) {
	now := time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC)
	svc := newRedisService(t, Config{
		Entities: []Entity{{ID: "series-1", EntityType: "series", Title: "Example Series"}},
		Clock:    stubClock{now: now},
		LibraryFiles: map[string][]LibraryFile{
			"series-1": {
				{
					ID:            "file-1",
					EntityID:      "series-1",
					WorkType:      "episode",
					RelativePath:  "Example Series/Season 01/Example Series - S01E02 - Second Episode.mkv",
					Extension:     "mkv",
					Quality:       "WEBDL-1080p",
					SeasonNumber:  1,
					EpisodeNumber: 2,
					EpisodeTitle:  "Second Episode",
					SeriesTitle:   "Example Series",
					ReleaseYear:   2024,
				},
			},
		},
	})

	preview, err := svc.PreviewRename(context.Background(), "series-1")
	if err != nil {
		t.Fatalf("PreviewRename returned error: %v", err)
	}
	if preview.EntityType != "series" {
		t.Fatalf("unexpected entity type %s", preview.EntityType)
	}
	if preview.Changes != 1 {
		t.Fatalf("expected 1 pending rename, got %d", preview.Changes)
	}
	if preview.EpisodeTemplate == nil {
		t.Fatalf("expected episode template to be returned")
	}
	if len(preview.Operations) != 1 {
		t.Fatalf("expected one operation, got %d", len(preview.Operations))
	}
	op := preview.Operations[0]
	expectedTarget := "Example Series/Season 01/Example Series - S01E02 - Second Episode [WEBDL-1080p].mkv"
	if op.TargetPath != expectedTarget {
		t.Fatalf("unexpected target path: got %q want %q", op.TargetPath, expectedTarget)
	}
	if !op.ShouldRename {
		t.Fatalf("expected preview to recommend rename")
	}
}

func TestStartRenameJobAndHistory(t *testing.T) {
	now := time.Date(2025, 2, 2, 14, 45, 0, 0, time.UTC)
	svc := newRedisService(t, Config{
		Entities: []Entity{{ID: "series-1", EntityType: "series", Title: "History Show"}},
		Clock:    stubClock{now: now},
		LibraryFiles: map[string][]LibraryFile{
			"series-1": {
				{
					ID:            "file-1",
					EntityID:      "series-1",
					WorkType:      "episode",
					RelativePath:  "History Show/Season 01/History Show - S01E01 - Pilot.mkv",
					Extension:     "mkv",
					Quality:       "1080p WEBDL",
					SeasonNumber:  1,
					EpisodeNumber: 1,
					EpisodeTitle:  "Pilot",
					SeriesTitle:   "History Show",
					ReleaseYear:   2023,
				},
			},
		},
	})

	job, err := svc.StartRenameJob(context.Background(), "series-1", RenameOptions{})
	if err != nil {
		t.Fatalf("StartRenameJob returned error: %v", err)
	}
	if job.ID == "" {
		t.Fatalf("expected job id to be populated")
	}
	if job.Changes != 1 {
		t.Fatalf("expected a single rename, got %d", job.Changes)
	}
	if len(job.Operations) != 1 {
		t.Fatalf("expected single operation, got %d", len(job.Operations))
	}
	if !job.Operations[0].ShouldRename {
		t.Fatalf("expected operation to execute rename")
	}
	if job.Operations[0].LastRenamedJobID != job.ID {
		t.Fatalf("expected operation to record job id")
	}

	second, err := svc.StartRenameJob(context.Background(), "series-1", RenameOptions{})
	if err != nil {
		t.Fatalf("StartRenameJob second run returned error: %v", err)
	}
	if second.Changes != 0 {
		t.Fatalf("expected second run to detect no changes, got %d", second.Changes)
	}
	if got := second.Operations[0].Reason; !strings.Contains(got, job.ID) {
		t.Fatalf("expected reason to reference first job id, got %q", got)
	}

	history, err := svc.RenameHistory(context.Background(), "series-1", 10)
	if err != nil {
		t.Fatalf("RenameHistory returned error: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(history))
	}
	if !history[0].RequestedAt.After(history[1].RequestedAt) && !history[0].RequestedAt.Equal(history[1].RequestedAt) {
		t.Fatalf("expected history to be sorted by newest first")
	}
}

func TestConfigureRenameTemplatesOverridesMovie(t *testing.T) {
	svc := newRedisService(t, Config{
		Entities: []Entity{{ID: "movie-1", EntityType: "movie", Title: "Amazing Film"}},
		LibraryFiles: map[string][]LibraryFile{
			"movie-1": {
				{
					ID:           "movie-file-1",
					EntityID:     "movie-1",
					WorkType:     "movie",
					RelativePath: "Amazing Film/Amazing Film (2024).mkv",
					Extension:    "mkv",
					MovieTitle:   "Amazing Film",
					Quality:      "UHD REMUX",
					ReleaseYear:  2024,
				},
			},
		},
	})

	templates := RenameTemplates{
		Movie: MovieTemplateSet{
			MovieFolderPattern: "Movies/{MovieTitle}",
			MovieFilePattern:   "{MovieTitle} - {Quality}.{Extension:lower}",
		},
	}
	if _, err := svc.ConfigureRenameTemplates(context.Background(), templates); err != nil {
		t.Fatalf("ConfigureRenameTemplates returned error: %v", err)
	}

	preview, err := svc.PreviewRename(context.Background(), "movie-1")
	if err != nil {
		t.Fatalf("PreviewRename returned error: %v", err)
	}
	expected := "Movies/Amazing Film/Amazing Film - UHD REMUX.mkv"
	if preview.Operations[0].TargetPath != expected {
		t.Fatalf("unexpected movie target path: got %q want %q", preview.Operations[0].TargetPath, expected)
	}
}

func TestDryRunDoesNotPersistRename(t *testing.T) {
	svc := newRedisService(t, Config{
		Entities: []Entity{{ID: "artist-1", EntityType: "artist", Title: "Demo Artist"}},
		LibraryFiles: map[string][]LibraryFile{
			"artist-1": {
				{
					ID:           "track-1",
					EntityID:     "artist-1",
					WorkType:     "track",
					RelativePath: "Demo Artist/Unsorted/track1.flac",
					Extension:    "flac",
					ArtistName:   "Demo Artist",
					AlbumTitle:   "Debut Album",
					TrackTitle:   "Opening Track",
					TrackNumber:  1,
					ReleaseYear:  2022,
				},
			},
		},
	})

	job, err := svc.StartRenameJob(context.Background(), "artist-1", RenameOptions{DryRun: true})
	if err != nil {
		t.Fatalf("StartRenameJob dry run returned error: %v", err)
	}
	if !job.DryRun {
		t.Fatalf("expected dry run flag to be true")
	}
	if job.Changes != 1 {
		t.Fatalf("expected pending rename count to be 1, got %d", job.Changes)
	}

	history, err := svc.RenameHistory(context.Background(), "artist-1", 5)
	if err != nil {
		t.Fatalf("RenameHistory returned error: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("expected no history for dry run, got %d", len(history))
	}

	preview, err := svc.PreviewRename(context.Background(), "artist-1")
	if err != nil {
		t.Fatalf("PreviewRename after dry run returned error: %v", err)
	}
	op := preview.Operations[0]
	if op.LastRenamedJobID != "" {
		t.Fatalf("expected no rename history for dry run, got %q", op.LastRenamedJobID)
	}
}
