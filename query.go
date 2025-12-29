package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/go-github/v80/github"
	"go.uber.org/zap"
)

type Query struct {
	SearchTerm string
	Desc       string
}

func QueryRepositories(
	ctx context.Context,
	ghClient *github.Client,
	logger *zap.Logger,
	bufferSize int,
) <-chan *github.Repository {
	var queriesWg sync.WaitGroup

	out := make(chan *github.Repository, bufferSize)

	for _, q := range initQueries() {
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

func initQueries() []Query {
	return []Query{
		Query{
			SearchTerm: "OPENAI_API_KEY=sk",
			Desc:       "Search for OpenAI key",
		},
		Query{
			SearchTerm: "filename:.env TWILIO_AUTH_TOKEN",
			Desc:       "Searches for 'TWILIO_AUTH_TOKEN' in `.env` files, which may reveal Twilio authentication tokens.",
		},
		Query{
			SearchTerm: "filename:.env TWILIO_ACCOUNT_SID",
			Desc:       "Searches for 'TWILIO_ACCOUNT_SID' in `.env` files, which may expose Twilio account SID.",
		},
		Query{
			SearchTerm: "filename:.env TWILIO_API_KEY",
			Desc:       "Searches for 'TWILIO_API_KEY' in `.env` files, which may reveal Twilio API keys.",
		},
		Query{
			SearchTerm: "filename:.env TWILIO_PHONE_NUMBER",
			Desc:       "Searches for 'TWILIO_PHONE_NUMBER' in `.env` files, which may expose Twilio phone numbers.",
		},
		Query{
			SearchTerm: "filename:.env api_key",
			Desc:       "Searches for 'api_key' in `.env` files, which may reveal API keys for external services.",
		},
		Query{
			SearchTerm: "filename:.env password OR secret",
			Desc:       "Searches for 'password' or 'secret' in `.env` files, commonly used to store sensitive credentials.",
		},
		Query{
			SearchTerm: "AWS_ACCESS_KEY_ID OR AWS_SECRET_ACCESS_KEY filename:.env",
			Desc:       "Searches for AWS access and secret keys within `.env` files.",
		},
		Query{
			SearchTerm: "ghp_ filename:.env",
			Desc:       "Searches for GitHub Personal Access Tokens (PATs), which start with `ghp_`.",
		},
		Query{
			SearchTerm: "filename:.env jwt OR bearer",
			Desc:       "Searches for JWT (JSON Web Token) or bearer tokens in `.env` files.",
		},
		Query{
			SearchTerm: "filename:.env OR filename:config.yml OR filename:.bashrc OR filename:settings.py token",
			Desc:       "Searches for the word 'token' in several common configuration files that might store secrets.",
		},
		Query{
			SearchTerm: "filename:*.yml path:.github/workflows secret",
			Desc:       "Searches for secrets in GitHub Actions workflow files, which are often stored in `.yml` files under `.github/workflows`.",
		},
		Query{
			SearchTerm: "filename:.env API_KEY OR SECRET_KEY OR DB_PASSWORD",
			Desc:       "Searches for common environment variable names in `.env` files, which may expose API keys, secret keys, or database passwords.",
		},
		Query{
			SearchTerm: "filename:.env oauth OR token",
			Desc:       "Searches for OAuth tokens in `.env` files, which might be used for authentication in OAuth-based systems.",
		},
	}
}
