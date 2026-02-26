package market

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"brale-core/internal/pkg/logging"
	"brale-core/internal/snapshot"

	"go.uber.org/zap"
)

const (
	fearGreedEndpoint       = "https://api.alternative.me/fng/?limit=5"
	fearGreedErrorBackoff   = 2 * time.Minute
	fearGreedFallbackUpdate = 12 * time.Hour
)

type FearGreedPoint struct {
	Value          int
	Classification string
	Timestamp      time.Time
}

type FearGreedData struct {
	Value           int
	Classification  string
	Timestamp       time.Time
	TimeUntilUpdate time.Duration
	History         []FearGreedPoint
	LastUpdate      time.Time
	Error           string
}

type FearGreedService struct {
	endpoint string
	client   *http.Client

	mu         sync.RWMutex
	data       FearGreedData
	nextUpdate time.Time
	refreshMu  sync.Mutex
}

func NewFearGreedService() *FearGreedService {
	return &FearGreedService{
		endpoint: fearGreedEndpoint,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (s *FearGreedService) Get() (FearGreedData, bool) {
	if s == nil {
		return FearGreedData{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ok := !s.data.LastUpdate.IsZero()
	return s.data, ok
}

func (s *FearGreedService) RefreshIfStale(ctx context.Context) {
	if s == nil {
		return
	}
	now := time.Now()
	s.mu.RLock()
	next := s.nextUpdate
	last := s.data.LastUpdate
	s.mu.RUnlock()
	if !last.IsZero() && !next.IsZero() && now.Before(next) {
		return
	}

	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	s.mu.RLock()
	next = s.nextUpdate
	last = s.data.LastUpdate
	s.mu.RUnlock()
	if !last.IsZero() && !next.IsZero() && now.Before(next) {
		return
	}
	if err := s.refresh(ctx); err != nil {
		logging.FromContext(ctx).Named("market").Warn("fear_greed refresh failed", zap.Error(err))
	}
}

func (s *FearGreedService) FearGreed(ctx context.Context) (snapshot.FearGreedPoint, error) {
	if s == nil {
		return snapshot.FearGreedPoint{}, fmt.Errorf("fear_greed service not initialized")
	}
	s.RefreshIfStale(ctx)
	if data, ok := s.Get(); ok && !data.Timestamp.IsZero() {
		return snapshot.FearGreedPoint{Value: float64(data.Value), Timestamp: data.Timestamp.Unix()}, nil
	}
	if err := s.refresh(ctx); err != nil {
		return snapshot.FearGreedPoint{}, err
	}
	data, ok := s.Get()
	if !ok || data.Timestamp.IsZero() {
		return snapshot.FearGreedPoint{}, fmt.Errorf("fear_greed data unavailable")
	}
	return snapshot.FearGreedPoint{Value: float64(data.Value), Timestamp: data.Timestamp.Unix()}, nil
}

type fearGreedResponse struct {
	Data []struct {
		Value               string `json:"value"`
		ValueClassification string `json:"value_classification"`
		Timestamp           string `json:"timestamp"`
		TimeUntilUpdate     string `json:"time_until_update"`
	} `json:"data"`
	Metadata struct {
		Error interface{} `json:"error"`
	} `json:"metadata"`
}

func (s *FearGreedService) refresh(ctx context.Context) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("fear_greed service not initialized")
	}
	payload, err := s.fetchPayload(ctx)
	if err != nil {
		s.setError(err)
		return err
	}
	points, until, err := parseFearGreedPoints(payload)
	if err != nil {
		s.setError(err)
		return err
	}
	if len(points) == 0 {
		return fmt.Errorf("fear_greed points empty")
	}
	data := FearGreedData{
		Value:           points[0].Value,
		Classification:  points[0].Classification,
		Timestamp:       points[0].Timestamp,
		TimeUntilUpdate: until,
		History:         points,
		LastUpdate:      time.Now(),
	}
	s.setData(data, time.Now().Add(until))
	return nil
}

func (s *FearGreedService) fetchPayload(ctx context.Context) (fearGreedResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.endpoint, nil)
	if err != nil {
		return fearGreedResponse{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return fearGreedResponse{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fearGreedResponse{}, fmt.Errorf("fear_greed unexpected status %s", resp.Status)
	}
	var payload fearGreedResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fearGreedResponse{}, err
	}
	return payload, nil
}

func parseFearGreedPoints(payload fearGreedResponse) ([]FearGreedPoint, time.Duration, error) {
	if payload.Metadata.Error != nil {
		return nil, 0, fmt.Errorf("fear_greed api error: %v", payload.Metadata.Error)
	}
	if len(payload.Data) == 0 {
		return nil, 0, fmt.Errorf("fear_greed data empty")
	}
	points := make([]FearGreedPoint, 0, len(payload.Data))
	var until time.Duration
	for idx, item := range payload.Data {
		val, err := strconv.Atoi(strings.TrimSpace(item.Value))
		if err != nil {
			return nil, 0, fmt.Errorf("fear_greed invalid value: %w", err)
		}
		ts, err := strconv.ParseInt(strings.TrimSpace(item.Timestamp), 10, 64)
		if err != nil {
			return nil, 0, fmt.Errorf("fear_greed invalid timestamp: %w", err)
		}
		if idx == 0 && item.TimeUntilUpdate != "" {
			sec, err := strconv.ParseInt(strings.TrimSpace(item.TimeUntilUpdate), 10, 64)
			if err == nil && sec > 0 {
				until = time.Duration(sec) * time.Second
			}
		}
		points = append(points, FearGreedPoint{
			Value:          val,
			Classification: strings.TrimSpace(item.ValueClassification),
			Timestamp:      time.Unix(ts, 0).UTC(),
		})
	}
	return points, until, nil
}

func (s *FearGreedService) setError(err error) {
	if s == nil {
		return
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	data := FearGreedData{
		Error:      msg,
		LastUpdate: time.Now(),
	}
	s.setData(data, time.Now().Add(fearGreedErrorBackoff))
}

func (s *FearGreedService) setData(data FearGreedData, next time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = data
	if next.IsZero() {
		next = time.Now().Add(fearGreedFallbackUpdate)
	}
	s.nextUpdate = next
}
