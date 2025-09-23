package grpcapi

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/arrgo/arrgo/internal/api/librarypb"
	"github.com/arrgo/arrgo/internal/api/workerpb"
	"github.com/arrgo/arrgo/internal/library"
	"github.com/arrgo/arrgo/internal/worker"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

type stubClock struct{ now time.Time }

func (s stubClock) Now() time.Time { return s.now }

func mustLibraryService(t *testing.T, cfg library.Config) *library.Service {
	t.Helper()

	svc, err := library.NewService(cfg)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}
	return svc
}

func mustRedisBackedService(t *testing.T, cfg library.Config) *library.Service {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cfg.RedisClient = client

	svc, err := library.NewService(cfg)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	t.Cleanup(func() {
		_ = client.Close()
	})

	return svc
}

func mustWorkerService(t *testing.T) *worker.Service {
	t.Helper()

	repo := worker.NewInMemoryRepository()
	metrics := worker.NewMetrics(prometheus.NewRegistry())
	svc, err := worker.NewService(worker.Config{Repository: repo, Metrics: metrics})
	if err != nil {
		t.Fatalf("worker.NewService returned error: %v", err)
	}
	return svc
}

func TestListLibraryEntities(t *testing.T) {
	svc := mustLibraryService(t, library.Config{Entities: []library.Entity{{ID: "1", EntityType: "series", Title: "Series"}}})
	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	librarypb.RegisterLibraryServiceServer(grpcServer, NewServer(svc, nil))
	go grpcServer.Serve(lis)

	conn, err := grpc.DialContext(context.Background(), "bufnet", grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to dial bufnet: %v", err)
	}
	defer conn.Close()
	defer grpcServer.Stop()
	defer lis.Close()

	client := librarypb.NewLibraryServiceClient(conn)
	resp, err := client.ListLibraryEntities(context.Background(), &librarypb.ListLibraryEntitiesRequest{})
	if err != nil {
		t.Fatalf("ListLibraryEntities returned error: %v", err)
	}
	if len(resp.GetEntities()) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(resp.GetEntities()))
	}
}

func TestTriggerJob(t *testing.T) {
	svc := mustRedisBackedService(t, library.Config{
		Entities: []library.Entity{{ID: "entity-1", EntityType: "movie"}},
		Clock:    stubClock{now: time.Unix(1700000000, 0)},
		IDGenerator: func() string {
			return "job-grpc"
		},
	})
	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	librarypb.RegisterLibraryServiceServer(grpcServer, NewServer(svc, nil))
	go grpcServer.Serve(lis)

	conn, err := grpc.DialContext(context.Background(), "bufnet", grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to dial bufnet: %v", err)
	}
	defer conn.Close()
	defer grpcServer.Stop()
	defer lis.Close()

	client := librarypb.NewLibraryServiceClient(conn)
	resp, err := client.TriggerJob(context.Background(), &librarypb.TriggerJobRequest{JobType: "refresh", TargetId: "entity-1"})
	if err != nil {
		t.Fatalf("TriggerJob returned error: %v", err)
	}
	if resp.GetJob().GetId() != "job-grpc" {
		t.Fatalf("expected job-grpc id, got %s", resp.GetJob().GetId())
	}
}

func TestWorkerRegisterAndHeartbeat(t *testing.T) {
	libSvc := mustLibraryService(t, library.Config{})
	workerSvc := mustWorkerService(t)
	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	server := NewServer(libSvc, workerSvc)
	librarypb.RegisterLibraryServiceServer(grpcServer, server)
	workerpb.RegisterWorkerServiceServer(grpcServer, server)
	go grpcServer.Serve(lis)

	conn, err := grpc.DialContext(context.Background(), "bufnet", grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to dial bufnet: %v", err)
	}
	defer conn.Close()
	defer grpcServer.Stop()
	defer lis.Close()

	workerClient := workerpb.NewWorkerServiceClient(conn)
	regResp, err := workerClient.RegisterWorker(context.Background(), &workerpb.WorkerRegistrationRequest{Pool: "default"})
	if err != nil {
		t.Fatalf("RegisterWorker returned error: %v", err)
	}
	if regResp.GetWorker().GetId() == "" {
		t.Fatalf("expected worker id in registration response")
	}

	hbResp, err := workerClient.Heartbeat(context.Background(), &workerpb.HeartbeatRequest{WorkerId: regResp.GetWorker().GetId(), ActiveJobs: 2})
	if err != nil {
		t.Fatalf("Heartbeat returned error: %v", err)
	}
	if hbResp.GetStale() {
		t.Fatalf("expected heartbeat to return healthy worker")
	}
}
