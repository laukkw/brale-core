package main

import (
	"net/http"
	"time"

	"brale-core/internal/transport/botruntime"
)

func newRuntimeClient() (*botruntime.Client, error) {
	return botruntime.NewClient(flagEndpoint, &http.Client{Timeout: 15 * time.Second})
}
