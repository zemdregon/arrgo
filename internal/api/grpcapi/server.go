package grpcapi

import (
	"context"
	"time"

	"github.com/arrgo/arrgo/internal/api/librarypb"
	"github.com/arrgo/arrgo/internal/api/workerpb"
	"github.com/arrgo/arrgo/internal/library"
	"github.com/arrgo/arrgo/internal/worker"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server adapts the shared library service for gRPC consumers.
type Server struct {
	librarypb.UnimplementedLibraryServiceServer
	workerpb.UnimplementedWorkerServiceServer

	libraryService *library.Service
	workerService  *worker.Service
}

// NewServer constructs a gRPC adapter around the library service.
func NewServer(libraryService *library.Service, workerService *worker.Service) *Server {
	return &Server{libraryService: libraryService, workerService: workerService}
}

// ListLibraryEntities returns catalog entities filtered by type.
func (s *Server) ListLibraryEntities(ctx context.Context, req *librarypb.ListLibraryEntitiesRequest) (*librarypb.ListLibraryEntitiesResponse, error) {
	entities, err := s.libraryService.ListEntities(ctx, library.ListEntitiesFilter{EntityType: req.GetEntityType()})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list entities: %v", err)
	}

	out := make([]*librarypb.LibraryEntity, 0, len(entities))
	for _, entity := range entities {
		out = append(out, &librarypb.LibraryEntity{
			Id:              entity.ID,
			EntityType:      entity.EntityType,
			Title:           entity.Title,
			Synopsis:        entity.Synopsis,
			PendingImports:  int32(entity.PendingImports),
			QualityUpgrades: int32(entity.QualityUpgrades),
			Failures:        int32(entity.Failures),
		})
	}

	return &librarypb.ListLibraryEntitiesResponse{Entities: out}, nil
}

// ListJobs returns currently queued jobs ordered by submission time.
func (s *Server) ListJobs(ctx context.Context, req *librarypb.ListJobsRequest) (*librarypb.ListJobsResponse, error) {
	jobs, err := s.libraryService.ListJobs(ctx)
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
	job, err := s.libraryService.TriggerJob(ctx, library.TriggerJobInput{
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

// RegisterWorker implements the worker handshake for gRPC clients.
func (s *Server) RegisterWorker(ctx context.Context, req *workerpb.WorkerRegistrationRequest) (*workerpb.WorkerRegistrationResponse, error) {
	if s.workerService == nil {
		return nil, status.Error(codes.Unavailable, "worker service unavailable")
	}

	result, err := s.workerService.RegisterWorker(ctx, worker.RegistrationRequest{
		Pool:         req.GetPool(),
		Capabilities: req.GetCapabilities(),
		Endpoint:     req.GetEndpoint(),
		Version:      req.GetVersion(),
	})
	if err != nil {
		code := codes.Internal
		if err == worker.ErrInvalidPool {
			code = codes.InvalidArgument
		}
		return nil, status.Error(code, err.Error())
	}

	return &workerpb.WorkerRegistrationResponse{
		Worker:     toProtoWorker(result.Worker),
		Assignment: toProtoAssignment(result.Assignment),
	}, nil
}

// Heartbeat updates worker availability state for gRPC clients.
func (s *Server) Heartbeat(ctx context.Context, req *workerpb.HeartbeatRequest) (*workerpb.HeartbeatResponse, error) {
	if s.workerService == nil {
		return nil, status.Error(codes.Unavailable, "worker service unavailable")
	}

	result, err := s.workerService.RecordHeartbeat(ctx, worker.HeartbeatInput{
		WorkerID:   req.GetWorkerId(),
		ActiveJobs: int(req.GetActiveJobs()),
	})
	if err != nil {
		switch err {
		case worker.ErrInvalidWorkerID:
			return nil, status.Error(codes.InvalidArgument, err.Error())
		case worker.ErrUnknownWorker:
			return nil, status.Error(codes.NotFound, err.Error())
		default:
			return nil, status.Errorf(codes.Internal, "heartbeat: %v", err)
		}
	}

	return &workerpb.HeartbeatResponse{
		Worker: toProtoWorker(result.Worker),
		Stale:  result.Stale,
	}, nil
}

func toProtoWorker(worker worker.Worker) *workerpb.WorkerDescriptor {
	return &workerpb.WorkerDescriptor{
		Id:              worker.ID,
		Pool:            worker.Pool,
		Capabilities:    append([]string(nil), worker.Capabilities...),
		Endpoint:        worker.Endpoint,
		Version:         worker.Version,
		Status:          string(worker.Status),
		RegisteredAt:    worker.RegisteredAt.Format(time.RFC3339),
		LastHeartbeatAt: worker.LastHeartbeatAt.Format(time.RFC3339),
		ActiveJobs:      int32(worker.ActiveJobs),
	}
}

func toProtoAssignment(assign worker.Assignment) *workerpb.WorkerAssignment {
	return &workerpb.WorkerAssignment{
		Stream:                   assign.Stream,
		ConsumerGroup:            assign.ConsumerGroup,
		HeartbeatIntervalSeconds: int64(assign.HeartbeatInterval / time.Second),
	}
}
