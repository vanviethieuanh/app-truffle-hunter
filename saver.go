package main

import (
	"context"
	"encoding/json"
	"os"

	"go.uber.org/zap"
)

func SaveToJsonl(
	ctx context.Context,
	in <-chan *Secret,
	logger *zap.Logger,
	filename string,
) {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		logger.Fatal("failed to open output file", zap.Error(err))
	}
	defer file.Close()

	encoder := json.NewEncoder(file)

	for {
		select {
		case <-ctx.Done():
			logger.Info("saver context cancelled", zap.Error(ctx.Err()))
			return
		case sec, ok := <-in:
			if !ok {
				logger.Info("result channel closed, saver exiting")
				return
			}

			if sec == nil {
				continue
			}

			if err := encoder.Encode(sec); err != nil {
				logger.Warn("failed to write scan result",
					zap.Error(err),
					zap.String("repo", sec.SourceMetadata.Data.Github.Repository),
				)
			}
		}
	}
}
