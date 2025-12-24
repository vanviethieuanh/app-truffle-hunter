package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/go-github/v80/github"
	"go.uber.org/zap"
)

type Scanner struct {
	Logger    *zap.Logger
	Semaphore chan struct{}

	scannedRepos  sync.Map
	scannedOwners sync.Map
}

func NewScanner(
	logger *zap.Logger,
	MaxConcurrentWorkers int,
	ctx context.Context,
) *Scanner {
	s := &Scanner{
		Logger:    logger,
		Semaphore: make(chan struct{}, MaxConcurrentWorkers),
	}

	loadSyncMap(&s.scannedRepos, "scanned_repos.json")
	loadSyncMap(&s.scannedOwners, "scanned_owners.json")

	s.startPeriodicDump(ctx, 1*time.Minute)

	return s
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

				skipped, verified, unverified, err := s.scanRepo(ctx, r, out)
				if skipped {
					return
				}
				if err != nil {
					s.Logger.Warn(
						"scan failed",
						zap.String("repo", r.GetFullName()),
						zap.Error(err),
					)
					return
				}

				if verified <= 0 {
					s.Logger.Info("finished scanning repo",
						zap.String("repo", r.GetFullName()),
						zap.Int("unverified", unverified),
						zap.Int("verified", verified),
					)
					return
				}

				s.Logger.Info(
					"Found verified secret(s) from a repo, starting scan an org",
					zap.String("owner", *r.Owner.Login),
				)
				s.scanOrg(ctx, *r.Owner.Login, out)
			}(repo)
		}
	}

	wg.Wait()
}

func (s *Scanner) scanRepo(
	ctx context.Context,
	r *github.Repository,
	out chan<- *ScanResult,
) (skipped bool, verified int, unverified int, err error) {
	_, loaded := s.scannedRepos.LoadOrStore(r.GetFullName(), struct{}{})
	if loaded {
		return loaded, 0, 0, nil
	}

	verified, unverified, err = s.runTrufflehog(ctx, out, "--repo", r.GetHTMLURL())
	return loaded, verified, unverified, err
}

func (s *Scanner) scanOrg(
	ctx context.Context,
	orgLogin string,
	out chan<- *ScanResult,
) (skipped bool, verified int, unverified int, err error) {
	_, loaded := s.scannedOwners.LoadOrStore(orgLogin, struct{}{})
	if loaded {
		return loaded, 0, 0, nil
	}

	verified, unverified, err = s.runTrufflehog(ctx, out, "--org", orgLogin)
	return loaded, verified, unverified, err
}

func (s *Scanner) runTrufflehog(
	ctx context.Context,
	out chan<- *ScanResult,
	args ...string,
) (verifiedCount, unverifiedCount int, err error) {
	target := "unknown"
	if len(args) >= 2 {
		target = args[1]
	}
	logger := s.Logger.With(zap.String("target", target))

	cmdArgs := append([]string{"github"}, args...)
	cmdArgs = append(cmdArgs,
		"--results", "verified,unverified,unknown",
		"--json",
		"--clone-path", "/mnt/ssd/tmp/trufflehog",
		"--log-level=-1",
		"--no-color",
		"--no-update",
	)
	cmd := exec.CommandContext(ctx, "trufflehog", cmdArgs...)

	logger.Info("Starting scan process.", zap.String("cmd", cmd.String()))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		logger.Warn("trufflehog failed",
			zap.Error(err),
			zap.String("stderr", stderr.String()),
		)
		return 0, 0, err
	}

	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var res ScanResult
		if err := json.Unmarshal(line, &res); err != nil {
			logger.Warn("failed to parse trufflehog line",
				zap.Error(err),
				zap.ByteString("line", line),
			)
			continue
		}

		select {
		case out <- &res:
			if res.Verified {
				verifiedCount++
			} else {
				unverifiedCount++
			}
		case <-ctx.Done():
			logger.Info("context cancelled while sending result")
			return verifiedCount, unverifiedCount, ctx.Err()
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Warn("error scanning trufflehog stdout",
			zap.Error(err),
		)
	}

	return verifiedCount, unverifiedCount, nil
}

func dumpSyncMap(m *sync.Map, filename string) error {
	keys := []string{}

	m.Range(func(key, value any) bool {
		if k, ok := key.(string); ok {
			keys = append(keys, k)
		}
		return true
	})

	data, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return err
	}

	filePath := filepath.Join(".cache", filename)
	return os.WriteFile(filePath, data, 0o644)
}

func loadSyncMap(m *sync.Map, filename string) error {
	filePath := filepath.Join(".cache", filename)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	var keys []string
	if err := json.Unmarshal(data, &keys); err != nil {
		return err
	}

	for _, k := range keys {
		m.Store(k, struct{}{})
	}

	return nil
}

func (s *Scanner) startPeriodicDump(ctx context.Context, interval time.Duration) {
	dumps := map[*sync.Map]string{
		&s.scannedRepos:  "scanned_repos.json",
		&s.scannedOwners: "scanned_owners.json",
	}

	ticker := time.NewTicker(interval)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				for m, file := range dumps {
					if err := dumpSyncMap(m, file); err != nil {
						s.Logger.Warn("failed to dump sync.Map", zap.String("file", file), zap.Error(err))
					}
				}
			case <-ctx.Done():
				s.Logger.Info("stopping periodic sync.Map dump")
				return
			}
		}
	}()
}

func (s *Scanner) DumpMaps() {
	if err := dumpSyncMap(&s.scannedRepos, "scanned_repos.json"); err != nil {
		s.Logger.Warn("failed to dump scannedRepos", zap.Error(err))
	}
	if err := dumpSyncMap(&s.scannedOwners, "scanned_owners.json"); err != nil {
		s.Logger.Warn("failed to dump scannedOwners", zap.Error(err))
	}
}
