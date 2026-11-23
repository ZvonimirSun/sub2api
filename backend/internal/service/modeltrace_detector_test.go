package service

import (
	"context"
	"errors"
	"testing"
	"time"

	lmfpd "github.com/Ikaleio/lm-detector/sdk/go"
	"github.com/stretchr/testify/require"
)

func TestLMFPDOfflineScorerGenerateChallenges(t *testing.T) {
	scorer := newLMFPDOfflineScorer()
	for _, count := range []int{1, 2, 3, 4, 6} {
		challenges, err := scorer.GenerateChallenges(count)
		require.NoError(t, err)
		require.Len(t, challenges, count)
		for _, ch := range challenges {
			require.NotEmpty(t, ch.ID)
			require.NotEmpty(t, ch.Prompt)
			require.GreaterOrEqual(t, ch.ExpectedCount, 292)
			require.LessOrEqual(t, ch.ExpectedCount, 332)
		}
	}
	_, err := scorer.GenerateChallenges(0)
	require.Error(t, err)
}

func TestLMFPDOfflineScorerParseNumbers(t *testing.T) {
	scorer := newLMFPDOfflineScorer()
	nums := scorer.ParseNumbers("Sure: 12, 45, 78, 101, 200")
	require.Equal(t, []int{12, 45, 78, 101, 200}, nums)
}

func TestLMFPDOfflineScorerDetectorCachingAndFallback(t *testing.T) {
	first := &lmfpd.Detector{}
	second := &lmfpd.Detector{}
	loads := 0
	shouldFail := false

	scorer := &lmfpdOfflineScorer{
		reloadInterval: time.Hour,
		loadFunc: func(context.Context, lmfpd.LoadOptions) (*lmfpd.Detector, error) {
			loads++
			if shouldFail {
				return nil, errors.New("network unreachable")
			}
			if loads == 1 {
				return first, nil
			}
			return second, nil
		},
	}

	det, err := scorer.getDetector(context.Background())
	require.NoError(t, err)
	require.Same(t, first, det)
	require.Equal(t, 1, loads)

	// Cached instance is reused while within reloadInterval.
	det, err = scorer.getDetector(context.Background())
	require.NoError(t, err)
	require.Same(t, first, det)
	require.Equal(t, 1, loads)

	// Stale cache attempts refresh; failure falls back gracefully to cached detector.
	scorer.loadedAt = time.Now().Add(-2 * time.Hour)
	shouldFail = true
	det, err = scorer.getDetector(context.Background())
	require.NoError(t, err)
	require.Same(t, first, det)
	require.Equal(t, 2, loads)

	// Subsequent refresh succeeds and swaps detector.
	scorer.loadedAt = time.Now().Add(-2 * time.Hour)
	shouldFail = false
	det, err = scorer.getDetector(context.Background())
	require.NoError(t, err)
	require.Same(t, second, det)
	require.Equal(t, 3, loads)
}

func TestMapLMFPDAnalysis(t *testing.T) {
	now := time.Now().UTC()
	prob := 0.942

	scored := mapLMFPDAnalysis(&lmfpd.Analysis{
		Decision:    "scored",
		Prediction:  "claude-opus-4-6",
		Probability: &prob,
	}, nil, 1234, now)
	require.Empty(t, scored.Error)
	require.Equal(t, "claude-opus-4-6", scored.DetectedModel)
	require.Equal(t, "scored", scored.Decision)
	require.InDelta(t, 0.942, scored.Probability, 1e-9)
	require.Equal(t, int64(1234), scored.LatencyMs)

	partial := mapLMFPDAnalysis(&lmfpd.Analysis{
		Decision:    "partial",
		Prediction:  "gpt-5.4",
		Probability: nil,
	}, nil, 900, now)
	require.Empty(t, partial.Error)
	require.Equal(t, "gpt-5.4", partial.DetectedModel)
	require.Equal(t, "partial", partial.Decision)
	require.Zero(t, partial.Probability)

	notModel := mapLMFPDAnalysis(&lmfpd.Analysis{
		Decision:       "not_model",
		PredictionName: "Not a model reply",
		Evidence:       lmfpd.Evidence{Reason: "Numbers look program-generated."},
	}, nil, 500, now)
	require.Equal(t, "Unknown", notModel.DetectedModel)
	require.Equal(t, "not_model", notModel.Decision)
	require.Equal(t, "Numbers look program-generated.", notModel.Error)

	unscorable := mapLMFPDAnalysis(&lmfpd.Analysis{
		Decision: "unscorable",
		Evidence: lmfpd.Evidence{Reason: "No valid samples."},
	}, nil, 400, now)
	require.Equal(t, "Unknown", unscorable.DetectedModel)
	require.Equal(t, "unscorable", unscorable.Decision)
	require.Equal(t, "No valid samples.", unscorable.Error)

	nilAnalysis := mapLMFPDAnalysis(nil, nil, 100, now)
	require.Equal(t, "Unknown", nilAnalysis.DetectedModel)
	require.NotEmpty(t, nilAnalysis.Error)
}
