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

type ScanRequest struct {
	org  *string
	repo *github.Repository
}

type ScanResult struct {
	sec   *Secret
	owner *string
}

func scan(
	ctx context.Context,
	in <-chan *ScanRequest,
	bufSize int,
	workers int,
	logger *zap.Logger,
) (<-chan *Secret, <-chan *string) {
	out := make(chan *Secret, bufSize)
	org := make(chan *string, bufSize)

	logger = logger.With(zap.String("stage", "Scan Repo"))

	go func() {
		var wg sync.WaitGroup

		for range workers {
			wg.Go(func() {
				for {
					select {
					case <-ctx.Done():
						logger.Info(
							"scanner context cancelled",
							zap.Error(ctx.Err()),
						)
						return
					case scanReq, ok := <-in:
						if !ok {
							return
						}

						var secCh <-chan *Secret
						if scanReq.org != nil {
							secCh = runTrufflehog(ctx, bufSize, logger, "--org", *scanReq.org)
						}
						if scanReq.repo != nil {
							secCh = runTrufflehog(ctx, bufSize, logger, "--repo", scanReq.repo.GetHTMLURL())
						}

						verified := 0
						for sec := range secCh {
							if sec.Verified {
								verified++
							}

							out <- sec
						}

						if verified > 0 && scanReq.repo != nil {
							org <- scanReq.repo.GetOwner().Login
						}
					}
				}
			})
		}

		go func() {
			wg.Wait()
			close(org)
			close(out)
		}()
	}()

	return out, org
}

func runTrufflehog(
	ctx context.Context,
	bufSize int,
	logger *zap.Logger,
	args ...string,
) <-chan *Secret {
	out := make(chan *Secret, bufSize)

	go func() {
		defer close(out)

		target := "unknown"
		if len(args) >= 2 {
			target = args[1]
		}
		logger = logger.With(zap.String("target", target))

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
			return
		}

		scanner := bufio.NewScanner(&stdout)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}

			var res Secret
			if err := json.Unmarshal(line, &res); err != nil {
				logger.Warn("failed to parse trufflehog line",
					zap.Error(err),
					zap.ByteString("line", line),
				)
				continue
			}

			select {
			case out <- &res:
			case <-ctx.Done():
				logger.Info("context cancelled while sending result")
				return
			}
		}

		if err := scanner.Err(); err != nil {
			logger.Warn("error scanning trufflehog stdout",
				zap.Error(err),
			)
		}
	}()

	return out
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

func startPeriodicDump(
	ctx context.Context,
	interval time.Duration,
	data *sync.Map,
	filePath string,
	logger *zap.Logger,
) {
	ticker := time.NewTicker(interval)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := dumpSyncMap(data, filePath); err != nil {
					logger.Warn(
						"failed to dump sync.Map",
						zap.String("file", filePath),
						zap.Error(err),
					)
				}
			case <-ctx.Done():
				logger.Info("stopping periodic sync.Map dump")
				return
			}
		}
	}()
}
