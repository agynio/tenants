package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	authorizationv1 "github.com/agynio/organizations/.gen/go/agynio/api/authorization/v1"
	identityv1 "github.com/agynio/organizations/.gen/go/agynio/api/identity/v1"
	organizationsv1 "github.com/agynio/organizations/.gen/go/agynio/api/organizations/v1"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/agynio/organizations/internal/config"
	"github.com/agynio/organizations/internal/db"
	"github.com/agynio/organizations/internal/server"
	"github.com/agynio/organizations/internal/store"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("organizations-service: %v", err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("create connection pool: %w", err)
	}
	defer pool.Close()

	if err := db.ApplyMigrations(ctx, pool); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	authConn, err := grpc.NewClient(cfg.AuthorizationAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect to authorization: %w", err)
	}
	defer authConn.Close()

	identityConn, err := grpc.NewClient(cfg.IdentityAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect to identity: %w", err)
	}
	defer identityConn.Close()

	grpcServer := grpc.NewServer()
	serverInstance := server.New(
		store.New(pool),
		authorizationv1.NewAuthorizationServiceClient(authConn),
		identityv1.NewIdentityServiceClient(identityConn),
	)
	organizationsv1.RegisterOrganizationsServiceServer(grpcServer, serverInstance)

	lis, err := net.Listen("tcp", cfg.GRPCAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.GRPCAddress, err)
	}

	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()

	log.Printf("OrganizationsService listening on %s", cfg.GRPCAddress)

	if err := grpcServer.Serve(lis); err != nil {
		if errors.Is(err, grpc.ErrServerStopped) {
			return nil
		}
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
