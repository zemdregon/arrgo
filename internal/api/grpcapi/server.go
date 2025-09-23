package grpcapi

import (
	"context"
	"time"

	"github.com/arrgo/arrgo/internal/api/librarypb"
	"github.com/arrgo/arrgo/internal/library"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server adapts the shared library service for gRPC consumers.
type Server struct {
	librarypb.UnimplementedLibraryServiceServer

	service *library.Service
}

// NewServer constructs a gRPC adapter around the library service.
func NewServer(service *library.Service) *Server {
	return &Server{service: service}
}

// ListLibraryEntities returns catalog entities filtered by type.
func (s *Server) ListLibraryEntities(ctx context.Context, req *librarypb.ListLibraryEntitiesRequest) (*librarypb.ListLibraryEntitiesResponse, error) {
	entities, err := s.service.ListEntities(ctx, library.ListEntitiesFilter{EntityType: req.GetEntityType()})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list entities: %v", err)
	}

	out := make([]*librarypb.LibraryEntity, 0, len(entities))
	for _, entity := range entities {
		out = append(out, &librarypb.LibraryEntity{
			Id:         entity.ID,
			EntityType: entity.EntityType,
			Title:      entity.Title,
			Synopsis:   entity.Synopsis,
		})
	}

	return &librarypb.ListLibraryEntitiesResponse{Entities: out}, nil
}

// ListJobs returns currently queued jobs ordered by submission time.
func (s *Server) ListJobs(ctx context.Context, req *librarypb.ListJobsRequest) (*librarypb.ListJobsResponse, error) {
	jobs, err := s.service.ListJobs(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list jobs: %v", err)
	}

	out := make([]*librarypb.JobSummary, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, toProtoJob(job))
	}

	return &librarypb.ListJobsResponse{Jobs: out}, nil
}

// TriggerJob schedules a new orchestration job for a catalog entity.
func (s *Server) TriggerJob(ctx context.Context, req *librarypb.TriggerJobRequest) (*librarypb.TriggerJobResponse, error) {
	job, err := s.service.TriggerJob(ctx, library.TriggerJobInput{
		JobType:        req.GetJobType(),
		TargetID:       req.GetTargetId(),
		PayloadURI:     req.GetPayloadUri(),
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		switch err {
		case library.ErrInvalidJobType, library.ErrInvalidTarget:
			return nil, status.Error(codes.InvalidArgument, err.Error())
		case library.ErrUnknownEntity:
			return nil, status.Error(codes.NotFound, err.Error())
		default:
			return nil, status.Errorf(codes.Internal, "trigger job: %v", err)
		}
	}

	return &librarypb.TriggerJobResponse{Job: toProtoJob(job)}, nil
}

func toProtoJob(job library.Job) *librarypb.JobSummary {
	var lastAttempt string
	if !job.LastAttemptAt.IsZero() {
		lastAttempt = job.LastAttemptAt.Format(time.RFC3339)
	}

	return &librarypb.JobSummary{
		Id:             job.ID,
		JobType:        job.JobType,
		TargetId:       job.TargetID,
		State:          job.State,
		SubmittedAt:    job.SubmittedAt.Format(time.RFC3339),
		PayloadUri:     job.PayloadURI,
		IdempotencyKey: job.IdempotencyKey,
		Attempt:        int32(job.Attempt),
		LastAttemptAt:  lastAttempt,
	}
}
