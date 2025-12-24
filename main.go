package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/go-github/v80/github"
	"go.uber.org/zap"
)

func main() {
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer cancel()

	logger, err := zap.NewProduction()
	defer logger.Sync()
	if err != nil {
		panic(err)
	}

	repoCh := make(chan *github.Repository, 100)
	secCh := make(chan *ScanResult, 100)

	httpClient := &http.Client{}
	ghClient := github.NewClient(httpClient)

	scanner := NewScanner(logger, 5, ctx)
	defer scanner.DumpMaps()

	go func() {
		defer close(repoCh)
		QueryRepositories(ctx, ghClient, repoCh, logger)
	}()

	go func() {
		defer close(secCh)
		scanner.Scan(ctx, repoCh, secCh)
	}()

	SaveToJsonl(ctx, secCh, logger, "result.jsonl")
}
