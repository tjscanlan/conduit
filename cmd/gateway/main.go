package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/your-org/agent-observability/internal/health"
	pb "github.com/your-org/agent-observability/proto"
)

func main() {
	cfg := loadConfig()
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(health.Probe(cfg.HealthAddr, os.Stdout))
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Load mTLS credentials
	tlsCfg, err := loadMTLSConfig(cfg)
	if err != nil {
		logger.Error("failed to load mTLS config", "err", err)
		os.Exit(1)
	}

	creds := credentials.NewTLS(tlsCfg)
	srv := grpc.NewServer(
		grpc.Creds(creds),
		grpc.ChainUnaryInterceptor(unaryInterceptors(cfg.RateLimit)...),
	)

	// Register ingestion service
	handler, err := NewIngestionHandler(cfg, logger)
	if err != nil {
		logger.Error("failed to init ingestion handler", "err", err)
		os.Exit(1)
	}
	pb.RegisterIngestionServiceServer(srv, handler)

	lis, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		logger.Error("failed to bind", "addr", cfg.ListenAddr, "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go health.Serve(ctx, cfg.HealthAddr, health.Handler(map[string]health.Check{
		"nats": handler.publisher.Ready,
	}, logger), logger)

	go func() {
		logger.Info("gateway listening", "addr", cfg.ListenAddr)
		if err := srv.Serve(lis); err != nil {
			logger.Error("server error", "err", err)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down gateway")
	srv.GracefulStop()
}

func loadMTLSConfig(cfg *Config) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return nil, err
	}

	caCert, err := os.ReadFile(cfg.TLSCAFile)
	if err != nil {
		return nil, err
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, errors.New("failed to parse CA certificate")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caPool,
		// Require and verify client cert — this is what makes it mTLS
		ClientAuth: tls.RequireAndVerifyClientCert,
		MinVersion: tls.VersionTLS13,
	}, nil
}
