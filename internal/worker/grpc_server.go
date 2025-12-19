package worker

import (
	"fmt"
	"io"
	"log"
	"net"

	"github.com/vigneshSrinivasan2005/DistRAFT/internal/api"
	"github.com/vigneshSrinivasan2005/DistRAFT/internal/store"
	"google.golang.org/grpc"
)

const (
	// ModelChunkSize is the size of each model chunk in bytes (1MB)
	ModelChunkSize = 1024 * 1024
)

// MLWorkerServer implements api.MLWorkerService
type MLWorkerServer struct {
	api.UnimplementedMLWorkerServiceServer
	state *store.State
	port  int
}

// NewMLWorkerServer creates a new gRPC server for ML operations
func NewMLWorkerServer(state *store.State, port int) *MLWorkerServer {
	return &MLWorkerServer{
		state: state,
		port:  port,
	}
}

// StartGRPCServer starts the gRPC server on the given port
func (s *MLWorkerServer) StartGRPCServer(port int) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	api.RegisterMLWorkerServiceServer(grpcServer, s)

	log.Printf("🚀 gRPC server listening on port %d", port)
	return grpcServer.Serve(lis)
}

// GetModel streams model chunks to a worker
// This is called by workers to download the model shard for training
func (s *MLWorkerServer) GetModel(req *api.ModelRequest, stream grpc.ServerStreamingServer[api.ModelChunk]) error {
	log.Printf("📥 GetModel request received")

	// TODO: In a real implementation, this would:
	// 1. Load the model from disk or memory
	// 2. Split it into chunks
	// 3. Stream chunks to the worker
	// For now, return a simple placeholder response

	chunk := &api.ModelChunk{
		ChunkId: 0,
		Data:    []byte("placeholder model data"),
	}

	if err := stream.Send(chunk); err != nil {
		log.Printf("❌ Failed to send model chunk: %v", err)
		return err
	}

	log.Printf("✓ Model chunk sent (ChunkID: %d)", chunk.ChunkId)
	return nil
}

// SendGradients receives gradient chunks from a worker
// This is called by workers to upload computed gradients after training
func (s *MLWorkerServer) SendGradients(stream grpc.ClientStreamingServer[api.GradientChunk, api.Ack]) error {
	log.Printf("📤 SendGradients request received")

	var jobID string
	var workerID string
	gradientCount := 0

	// Receive all gradient chunks
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			// Client finished sending
			log.Printf("✓ Received %d gradient chunks from worker %s for job %s", gradientCount, workerID, jobID)
			break
		}
		if err != nil {
			log.Printf("❌ Error receiving gradient chunk: %v", err)
			return err
		}

		jobID = chunk.JobId
		workerID = chunk.WorkerId
		gradientCount++

		log.Printf("  [Gradient %d] Job: %s, Worker: %s, Data size: %d bytes", gradientCount, jobID, workerID, len(chunk.Data))
	}

	// Send acknowledgment
	ack := &api.Ack{
		Success: true,
	}

	if err := stream.SendAndClose(ack); err != nil {
		log.Printf("❌ Failed to send acknowledgment: %v", err)
		return err
	}

	log.Printf("✓ Acknowledgment sent for job %s", jobID)
	return nil
}
