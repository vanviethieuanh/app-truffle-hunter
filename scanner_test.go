package main_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-github/v80/github"
	main "github.com/vanviethieuanh/app-truffle-hunter"
	"go.uber.org/zap"
)

func TestScanner_Scan(t *testing.T) {
	logger, err := zap.NewDevelopmentConfig().Build()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name                 string
		logger               *zap.Logger
		ghClient             *github.Client
		MaxConcurrentWorkers int
		testRepos            []github.Repository
	}{
		{
			name:                 "scan trufflehog test_keys repo",
			logger:               logger,
			MaxConcurrentWorkers: 2,
			testRepos: []github.Repository{
				{
					FullName: github.Ptr("trufflesecurity/test_keys"),
					HTMLURL:  github.Ptr("https://github.com/trufflesecurity/test_keys"),
					Owner: &github.User{
						Login: github.Ptr("trufflesecurity"),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			repoCh := make(chan *github.Repository, 10)
			secCh := make(chan *main.ScanResult, 100)

			s := main.NewScanner(tt.logger, tt.MaxConcurrentWorkers)

			// Start scanner
			go func() {
				s.Scan(ctx, repoCh, secCh)
				close(secCh)
			}()

			// Feed repos
			go func() {
				for _, repo := range tt.testRepos {
					repoCh <- &repo
				}
				close(repoCh)
			}()

			found := 0

			for {
				select {
				case sec, ok := <-secCh:
					if !ok {
						goto DONE
					}
					found++
					logger.Info("secret received",
						zap.String("repo", sec.SourceMetadata.Data.Github.Repository),
						zap.String("detector", sec.DetectorName),
					)

				case <-ctx.Done():
					t.Fatal("test timed out")
				}
			}

		DONE:
			if found == 0 {
				t.Fatal("expected at least one secret, found none")
			}

			t.Logf("found %d secrets", found)
		})
	}
}
