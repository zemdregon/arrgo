package grpcapi

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/arrgo/arrgo/internal/api/librarypb"
	"github.com/arrgo/arrgo/internal/library"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

type stubClock struct{ now time.Time }

func (s stubClock) Now() time.Time { return s.now }

func TestListLibraryEntities(t *testing.T) {
	svc := library.NewService(library.Config{Entities: []library.Entity{{ID: "1", EntityType: "series", Title: "Series"}}})
	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	librarypb.RegisterLibraryServiceServer(grpcServer, NewServer(svc))
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
	svc := library.NewService(library.Config{
		Entities: []library.Entity{{ID: "entity-1", EntityType: "movie"}},
		Clock:    stubClock{now: time.Unix(1700000000, 0)},
		IDGenerator: func() string {
			return "job-grpc"
		},
	})
	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	librarypb.RegisterLibraryServiceServer(grpcServer, NewServer(svc))
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
