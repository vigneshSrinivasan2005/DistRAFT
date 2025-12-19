package worker

import (
	"context"
	"io"
	"log"
	"time"

	"github.com/vigneshSrinivasan2005/DistRAFT/internal/api"
	"google.golang.org/grpc"
)

// MLWorkerClient wraps the gRPC client for calling the ML service
type MLWorkerClient struct {
	conn   *grpc.ClientConn
	client api.MLWorkerServiceClient
}

// NewMLWorkerClient creates a new gRPC client connection to the ML service
func NewMLWorkerClient(serverAddr string) (*MLWorkerClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, serverAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, err
	}

	client := api.NewMLWorkerServiceClient(conn)
	log.Printf("✓ Connected to ML service at %s", serverAddr)

	return &MLWorkerClient{
		conn:   conn,
		client: client,
	}, nil
}

// Close closes the gRPC connection
func (c *MLWorkerClient) Close() error {
	return c.conn.Close()
}

// DownloadModel downloads model chunks from the server
// This is called by workers to get the model for training
func (c *MLWorkerClient) DownloadModel(ctx context.Context) ([]byte, error) {
	log.Printf("📥 Requesting model from server...")

	req := &api.ModelRequest{}
	stream, err := c.client.GetModel(ctx, req)
	if err != nil {
		log.Printf("❌ Failed to request model: %v", err)
		return nil, err
	}

	var modelData []byte
	chunkCount := 0

	// Receive all model chunks
	for {
		chunk := &api.ModelChunk{}
		err := stream.RecvMsg(chunk)
		if err == io.EOF {
			// Server finished sending
			log.Printf("✓ Model download complete: %d chunks, %d bytes total", chunkCount, len(modelData))
			break
		}
		if err != nil {
			log.Printf("❌ Error receiving model chunk: %v", err)
			return nil, err
		}

		modelData = append(modelData, chunk.Data...)
		chunkCount++
		log.Printf("  [Model Chunk %d] Received %d bytes (Total: %d bytes)", chunk.ChunkId, len(chunk.Data), len(modelData))
	}

	return modelData, nil
}

// UploadGradients uploads gradient chunks to the server
// This is called by workers to send computed gradients after training
func (c *MLWorkerClient) UploadGradients(ctx context.Context, jobID string, workerID string, gradientData [][]byte) error {
	log.Printf("📤 Uploading gradients for job %s from worker %s...", jobID, workerID)

	stream, err := c.client.SendGradients(ctx)
	if err != nil {
		log.Printf("❌ Failed to create gradient stream: %v", err)
		return err
	}

	// Send all gradient chunks
	for i, data := range gradientData {
		chunk := &api.GradientChunk{
			JobId:    jobID,
			WorkerId: workerID,
			Data:     data,
		}

		if err := stream.SendMsg(chunk); err != nil {
			log.Printf("❌ Failed to send gradient chunk %d: %v", i, err)
			return err
		}
		log.Printf("  [Gradient %d] Sent %d bytes", i, len(data))
	}

	// Close send side and wait for acknowledgment
	ack := &api.Ack{}
	_, err = stream.CloseAndRecv()
	if err != nil {
		log.Printf("❌ Failed to receive acknowledgment: %v", err)
		return err
	}

	log.Printf("✓ Gradients uploaded successfully, acknowledgment: %v", ack.Success)
	return nil
}
