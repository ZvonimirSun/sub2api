package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
)

type modelTraceObservationKey struct{}

func accountTestMayChangeAvailability(ctx context.Context) bool {
	observational, _ := ctx.Value(modelTraceObservationKey{}).(bool)
	return !observational
}

// RunModelTraceProbe executes a model probe through the unified account test pipeline,
// preserving identical client headers, TLS fingerprint, proxy routing, and upstream handling.
func (s *AccountTestService) RunModelTraceProbe(
	ctx context.Context,
	accountID int64,
	modelID string,
	prompt string,
	reasoningEffort string,
) (string, error) {
	ctx = context.WithValue(ctx, modelTraceObservationKey{}, true)
	ctx = WithHTTPUpstreamProbe(ctx)
	w := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(w)
	ginCtx.Request = (&http.Request{}).WithContext(ctx)
	ginCtx.Set(accountTestBackgroundKey, true)

	opts := AccountTestOptions{
		ReasoningEffort: reasoningEffort,
	}

	testErr := s.TestAccountConnection(ginCtx, accountID, modelID, prompt, AccountTestModeDefault, opts)
	body := w.Body.String()
	responseText, errMsg := parseTestSSEOutput(body)

	if testErr != nil || errMsg != "" {
		if errMsg != "" {
			return "", fmt.Errorf("%s", errMsg)
		}
		return "", testErr
	}

	return responseText, nil
}
