package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/go-github/v80/github"
	"go.uber.org/zap"
)

type Query struct {
	SearchTerm string `json:"search"`
	Desc       string `json:"description"`
}

func QueryRepoGenerator(
	ctx context.Context,
	ghClient *github.Client,
	queries []Query,
	logger *zap.Logger,
	bufferSize int,
) <-chan *github.Repository {
	var queriesWg sync.WaitGroup

	out := make(chan *github.Repository, bufferSize)

	for _, q := range queries {
		queriesWg.Add(1)
		go func(q Query) {
			defer queriesWg.Done()
			q.getRepositories(ctx, ghClient, out, logger)
		}(q)
	}

	go func() {
		queriesWg.Wait()
		close(out)
	}()

	return out
}

func (q *Query) getRepositories(
	ctx context.Context,
	client *github.Client,
	out chan<- *github.Repository,
	logger *zap.Logger,
) {
	logger.With(zap.String("query", q.Desc))

	opts := &github.SearchOptions{
		Sort:  "stars",
		Order: "desc",
		ListOptions: github.ListOptions{
			PerPage: 50,
		},
	}

	for {
		result, resp, err := client.Search.Repositories(
			ctx,
			q.SearchTerm,
			opts,
		)
		if err != nil {
			// ── Primary rate limit ─────────────────────
			var rateErr *github.RateLimitError
			if errors.As(err, &rateErr) {
				reset := rateErr.Rate.Reset.Time
				sleep := time.Until(reset) + time.Second

				logger.Warn("primary rate limit hit",
					zap.Int("used", rateErr.Rate.Used),
					zap.Int("limit", rateErr.Rate.Limit),
					zap.Duration("sleep", sleep),
				)

				time.Sleep(sleep)
				continue
			}

			// ── Secondary (abuse) rate limit ───────────
			var abuseErr *github.AbuseRateLimitError
			if errors.As(err, &abuseErr) {
				var wait time.Duration

				if abuseErr.RetryAfter != nil {
					wait = *abuseErr.RetryAfter
				} else {
					wait = 30 * time.Second
				}

				logger.Warn("secondary rate limit hit",
					zap.Duration("retry_after", wait),
				)

				time.Sleep(wait)
				continue
			}

			// ── Real error ─────────────────────────────
			logger.Error("github search failed",
				zap.String("query", q.SearchTerm),
				zap.Error(err),
			)
			return
		}

		for _, r := range result.Repositories {
			select {
			case out <- r:
			case <-ctx.Done():
				logger.Info("context cancelled",
					zap.Error(ctx.Err()),
				)
				return
			}
		}

		if resp.NextPage == 0 {
			logger.Info("search completed",
				zap.String("query", q.SearchTerm),
			)
			return
		}

		opts.Page = resp.NextPage
	}
}

func LoadQueries(filepath string) ([]Query, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var queries []Query
	if err := json.Unmarshal(data, &queries); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return queries, nil
}
