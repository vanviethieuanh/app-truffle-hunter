package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"sync"

	"github.com/google/go-github/v80/github"
	"go.uber.org/zap"
)

type Scanner struct {
	Logger    *zap.Logger
	Semaphore chan struct{}
}

func NewScanner(
	logger *zap.Logger,
	MaxConcurrentWorkers int,
) *Scanner {
	return &Scanner{
		Logger:    logger,
		Semaphore: make(chan struct{}, MaxConcurrentWorkers),
	}
}

func (s *Scanner) Scan(
	ctx context.Context,
	in <-chan *github.Repository,
	out chan<- *ScanResult,
) {
	var wg sync.WaitGroup

	for repo := range in {
		select {
		case <-ctx.Done():
			s.Logger.Info("scanner context cancelled", zap.Error(ctx.Err()))
			return
		case s.Semaphore <- struct{}{}:
			wg.Add(1)
			go func(r *github.Repository) {
				defer func() {
					<-s.Semaphore
					wg.Done()
				}()
				verified, unverified, err := s.scanRepo(ctx, r, out)
				if err != nil {
					s.Logger.Warn("scan failed", zap.String("repo", r.GetFullName()), zap.Error(err))
				}
				s.Logger.Info("finished scanning repo",
					zap.String("repo", r.GetFullName()),
					zap.Int("unverified", unverified),
					zap.Int("verified", verified),
				)
			}(repo)
		}
	}

	wg.Wait()
}

func (s *Scanner) scanRepo(
	ctx context.Context,
	r *github.Repository,
	out chan<- *ScanResult,
) (int, int, error) {
	cmd := exec.CommandContext(ctx,
		"trufflehog",
		"github",
		"--repo", r.GetHTMLURL(),
		"--results", "verified,unverified,unknown",
		"--clone-path", "/mnt/ssd/tmp/trufflehog",
		"--json",
		"--log-level=-1",
		"--no-color",
		"--no-update",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		s.Logger.Warn("trufflehog failed",
			zap.String("repo", r.GetFullName()),
			zap.Error(err),
			zap.String("stderr", stderr.String()),
		)
		return 0, 0, err
	}

	verifiedCount := 0
	unverifedCount := 0
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var res ScanResult
		if err := json.Unmarshal(line, &res); err != nil {
			s.Logger.Warn("failed to parse trufflehog line",
				zap.String("repo", r.GetFullName()),
				zap.Error(err),
				zap.ByteString("line", line),
			)
			continue
		}

		// send to channel
		select {
		case out <- &res:
			if res.Verified {
				verifiedCount++
			} else {
				unverifedCount++
			}
		case <-ctx.Done():
			s.Logger.Info("context cancelled while sending result",
				zap.String("repo", r.GetFullName()),
			)
			return verifiedCount, unverifedCount, ctx.Err()
		}
	}

	if err := scanner.Err(); err != nil {
		s.Logger.Warn("error scanning trufflehog stdout",
			zap.String("repo", r.GetFullName()),
			zap.Error(err),
		)
	}

	// return number of secrets found
	return verifiedCount, unverifedCount, nil
}
