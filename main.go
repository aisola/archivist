package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"go.uber.org/zap"

	"github.com/aisola/archivist/middleware"
)

var (
	bind string
	b2KeyID string
	b2KeyToken string
	b2BucketID string
)

func init() {
	if b := os.Getenv("ARCHIVIST_BIND"); b != "" {
		bind = b
	}

	b2KeyID = os.Getenv("ARCHIVIST_B2_KEY_ID")
	b2KeyToken = os.Getenv("ARCHIVIST_B2_KEY_TOKEN")
	b2BucketID = os.Getenv("ARCHIVIST_B2_BUCKET_ID")

	flag.StringVar(&bind, "bind", bind, "the interface to bind to")
	flag.StringVar(&b2KeyID, "b2-key-id", b2KeyID, "the b2 key id")
	flag.StringVar(&b2KeyToken, "b2-key-token", b2KeyToken, "the b2 key token")
	flag.StringVar(&b2BucketID, "b2-bucket-id", b2BucketID, "the b2 bucket id")
}

func initializeLogger() *zap.Logger {
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("failed to initialize logger")
	}

	return logger
}

func main() {
	flag.Parse()

	logger := initializeLogger()

	b2 := NewB2(
		logger,
		b2KeyID,
		b2KeyToken,
		b2BucketID,
	)

	handler := middleware.AddRequestID(middleware.LogRequests(
		logger,
		New(logger, b2),
	))

	logger.Info("I'm Listening",
		zap.String("bind", bind),
		zap.String("b2_key_id", b2KeyID),
		zap.String("b2_key_token", b2KeyToken),
		zap.String("b2_bucket_id", b2BucketID),
	)
	if err := http.ListenAndServe(bind, handler); err != nil {
		logger.Error("failed to serve on address", zap.Error(err), zap.String("bind", bind))
	}
}

