package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/google/go-github/v80/github"
	"go.uber.org/zap"
)

const ScannedOwnerCachePath = "scanned_owners.json"
const ScannedRepoCachePath = "scanned_repos.json"
const QueriesPath = "queries.json"

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

	httpClient := &http.Client{}
	ghClient := github.NewClient(httpClient)

	var scannedRepos sync.Map
	var scannedOwners sync.Map

	loadSyncMap(&scannedOwners, ScannedOwnerCachePath)
	loadSyncMap(&scannedRepos, ScannedRepoCachePath)

	startPeriodicDump(ctx, 1*time.Minute, &scannedOwners, ScannedOwnerCachePath, logger)
	startPeriodicDump(ctx, 1*time.Minute, &scannedRepos, ScannedRepoCachePath, logger)

	defer func() {
		dumpSyncMap(&scannedOwners, ScannedOwnerCachePath)
		dumpSyncMap(&scannedRepos, ScannedRepoCachePath)
	}()

	loadedQueries, err := LoadQueries(QueriesPath)
	if err != nil {
		logger.Error("Failed to load queries", zap.Error(err))
	}

	initScanReqCh := Mapper(
		ctx,
		QueryRepoGenerator(ctx, ghClient, loadedQueries, logger, 100),
		10,
		func(i *github.Repository) *ScanRequest {
			return &ScanRequest{
				repo: i,
			}
		},
	)
	feedbackOrgCh := make(chan *ScanRequest)

	mergedReqCh := Merge(ctx, 30, initScanReqCh, feedbackOrgCh)

	filteredScan := Filter(ctx, mergedReqCh, func(r *ScanRequest) bool {
		if r.org != nil {
			_, loaded := scannedOwners.LoadOrStore(r.org, struct{}{})
			return loaded
		}

		if r.repo != nil {
			_, loaded := scannedRepos.LoadOrStore(r.repo.GetHTMLURL(), struct{}{})
			return loaded
		}

		return false
	})
	secCh, orgCh := scan(ctx, filteredScan, 10, runtime.NumCPU(), logger)

	orgScanCh := Mapper(ctx, orgCh, 10, func(i *string) *ScanRequest {
		return &ScanRequest{
			org: i,
		}
	})
	go func() {
		defer close(feedbackOrgCh)

		for o := range orgScanCh {
			select {
			case <-ctx.Done():
				return
			case feedbackOrgCh <- o:
			}
		}
	}()

	SaveToJsonl(ctx, secCh, logger, "result.jsonl")
}
