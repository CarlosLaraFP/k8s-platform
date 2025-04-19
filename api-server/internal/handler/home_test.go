package handler

import (
	"api-server/internal/metrics"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	//"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestSubmitHandler_InvalidName(t *testing.T) {
	body := strings.NewReader("name=INVALID_NAME_TOO_LONG_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")
	req := httptest.NewRequest(http.MethodPost, "/submit", body)
	//req.Form.Set()
	rr := httptest.NewRecorder()

	h := &Handler{
		KubeClient: &KubeClient{}, // not used in this test
		Metrics:    metrics.InitPrometheus(),
	}

	h.SubmitHandler(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	//assert.Equal(t, 1, int(testutil.ToFloat64(h.Metrics.ClaimsFailed.WithLabelValues("region", "username"))))
}

// We can mock Kube.DynamicClient with github.com/stretchr/testify/mock later for deeper tests.
