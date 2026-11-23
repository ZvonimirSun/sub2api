package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	lmfpd "github.com/Ikaleio/lm-detector/sdk/go"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"golang.org/x/sync/singleflight"
)

const (
	modelTraceMinProbeCount          = 1
	modelTraceDefaultProbeCount      = 3
	modelTraceMaxProbeCount          = 3
	modelTraceDetectorReloadInterval = 6 * time.Hour
)

type modelTraceChallenge struct {
	ID            string
	ExpectedCount int
	Prompt        string
}

// modelTraceScorer abstracts the offline lmfpd challenge generation, number
// parsing, and scoring pipeline so service unit tests stay deterministic and
// completely offline.
type modelTraceScorer interface {
	GenerateChallenges(count int) ([]modelTraceChallenge, error)
	ParseNumbers(text string) []int
	Analyze(ctx context.Context, outputs []lmfpd.Output) (*lmfpd.Analysis, error)
}

type lmfpdOfflineScorer struct {
	loadOptions    lmfpd.LoadOptions
	reloadInterval time.Duration
	loadFunc       func(context.Context, lmfpd.LoadOptions) (*lmfpd.Detector, error)

	mu        sync.RWMutex
	detector  *lmfpd.Detector
	loadedAt  time.Time
	loadGroup singleflight.Group
}

func newLMFPDOfflineScorer() *lmfpdOfflineScorer {
	return &lmfpdOfflineScorer{
		reloadInterval: modelTraceDetectorReloadInterval,
		loadFunc:       lmfpd.Load,
	}
}

func (s *lmfpdOfflineScorer) GenerateChallenges(count int) ([]modelTraceChallenge, error) {
	if count <= 0 {
		return nil, fmt.Errorf("challenge count must be positive")
	}
	challenges := make([]modelTraceChallenge, 0, count)
	usedCounts := make(map[int]struct{}, count)
	for len(challenges) < count {
		batchSize := min(modelTraceMaxProbeCount, count-len(challenges))
		batch, err := lmfpd.GenerateChallenges(batchSize)
		if err != nil {
			return nil, err
		}
		added := 0
		for _, ch := range batch {
			if len(challenges) >= count {
				break
			}
			if _, dup := usedCounts[ch.ExpectedCount]; dup {
				continue
			}
			usedCounts[ch.ExpectedCount] = struct{}{}
			challenges = append(challenges, modelTraceChallenge{
				ID:            ch.ID,
				ExpectedCount: ch.ExpectedCount,
				Prompt:        ch.Prompt,
			})
			added++
		}
		if added == 0 {
			// Fallback if random batches collided with already selected counts.
			for _, ch := range batch {
				if len(challenges) >= count {
					break
				}
				challenges = append(challenges, modelTraceChallenge{
					ID:            ch.ID,
					ExpectedCount: ch.ExpectedCount,
					Prompt:        ch.Prompt,
				})
			}
		}
	}
	return challenges, nil
}

func (s *lmfpdOfflineScorer) ParseNumbers(text string) []int {
	return lmfpd.ParseNumbers(text)
}

func (s *lmfpdOfflineScorer) getDetector(ctx context.Context) (*lmfpd.Detector, error) {
	s.mu.RLock()
	cached := s.detector
	fresh := cached != nil && (s.reloadInterval <= 0 || time.Since(s.loadedAt) < s.reloadInterval)
	s.mu.RUnlock()
	if fresh {
		return cached, nil
	}

	loadFn := s.loadFunc
	if loadFn == nil {
		loadFn = lmfpd.Load
	}

	val, err, _ := s.loadGroup.Do("detector", func() (any, error) {
		s.mu.RLock()
		existing := s.detector
		if existing != nil && (s.reloadInterval <= 0 || time.Since(s.loadedAt) < s.reloadInterval) {
			s.mu.RUnlock()
			return existing, nil
		}
		s.mu.RUnlock()

		// Use a decoupled context with a bounded timeout so a single canceled
		// probe does not abort reference bank loading for other probes.
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
		defer cancel()

		det, loadErr := loadFn(loadCtx, s.loadOptions)
		if loadErr != nil {
			if existing != nil {
				logger.LegacyPrintf("service.modeltrace", "[ModelTrace] lmfpd reference bank refresh failed, continuing with cached bank: %v", loadErr)
				s.mu.Lock()
				s.loadedAt = time.Now()
				s.mu.Unlock()
				return existing, nil
			}
			return nil, loadErr
		}

		s.mu.Lock()
		s.detector = det
		s.loadedAt = time.Now()
		s.mu.Unlock()
		return det, nil
	})
	if err != nil {
		return nil, err
	}
	det, ok := val.(*lmfpd.Detector)
	if !ok || det == nil {
		return nil, fmt.Errorf("lmfpd detector unavailable")
	}
	return det, nil
}

func (s *lmfpdOfflineScorer) Analyze(ctx context.Context, outputs []lmfpd.Output) (*lmfpd.Analysis, error) {
	det, err := s.getDetector(ctx)
	if err != nil {
		return nil, fmt.Errorf("load lmfpd reference bank: %w", err)
	}
	if len(outputs) > modelTraceMaxProbeCount {
		outputs = outputs[:modelTraceMaxProbeCount]
	}
	return det.Analyze(outputs, false)
}

func mapLMFPDAnalysis(
	analysis *lmfpd.Analysis,
	requests []ModelTraceRequestRecord,
	latencyMs int64,
	testedAt time.Time,
) *ModelTraceRecord {
	if analysis == nil {
		return &ModelTraceRecord{
			DetectedModel: "Unknown",
			Requests:      requests,
			Probability:   0,
			LatencyMs:     latencyMs,
			TestedAt:      testedAt,
			Error:         "empty fingerprint analysis",
		}
	}

	decision := strings.TrimSpace(analysis.Decision)
	switch decision {
	case "scored", "partial":
		detected := strings.TrimSpace(analysis.Prediction)
		if detected == "" {
			detected = strings.TrimSpace(analysis.PredictionName)
		}
		if detected == "" {
			reason := strings.TrimSpace(analysis.Evidence.Reason)
			if reason == "" {
				reason = "fingerprint prediction unavailable"
			}
			return &ModelTraceRecord{
				DetectedModel: "Unknown",
				Decision:      decision,
				Requests:      requests,
				Probability:   0,
				LatencyMs:     latencyMs,
				TestedAt:      testedAt,
				Error:         reason,
			}
		}
		prob := 0.0
		if analysis.Probability != nil {
			prob = *analysis.Probability
		}
		return &ModelTraceRecord{
			DetectedModel: detected,
			Decision:      decision,
			Requests:      requests,
			Probability:   prob,
			LatencyMs:     latencyMs,
			TestedAt:      testedAt,
		}
	default:
		reason := strings.TrimSpace(analysis.Evidence.Reason)
		if reason == "" {
			if decision == "not_model" {
				reason = "reply does not match natural model number distribution"
			} else {
				reason = "unscorable model outputs"
			}
		}
		return &ModelTraceRecord{
			DetectedModel: "Unknown",
			Decision:      decision,
			Requests:      requests,
			Probability:   0,
			LatencyMs:     latencyMs,
			TestedAt:      testedAt,
			Error:         reason,
		}
	}
}
