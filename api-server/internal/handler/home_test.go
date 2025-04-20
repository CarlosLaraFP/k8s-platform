package handler

import (
	"api-server/internal/metrics"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	//"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestSubmitHandler_InvalidName(t *testing.T) {
	// Explicitly simulating what a browser does:
	// Encoding key/value pairs as a POST body
	// Setting the proper Content-Type header
	req := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(""))
	_ = req.ParseForm()
	req.Form.Set("type", "Storage")
	req.Form.Set("name", "INVALID_NAME_TOO_LONG_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")
	req.Form.Set("username", "dev")
	req.Form.Set("region", "US")
	rr := httptest.NewRecorder()

	h := &Handler{
		KubeClient: &KubeClient{}, // not used in this test
		Metrics:    metrics.InitPrometheus(),
	}

	h.SubmitHandler(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestSubmitHandler_InvalidResource(t *testing.T) {
	//body := strings.NewReader("name=myresource&username=dev&type=microservice")
	req := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(""))
	_ = req.ParseForm()
	req.Form.Set("type", "Microservice")
	req.Form.Set("name", "myresource")
	req.Form.Set("username", "dev")
	req.Form.Set("region", "US")
	rr := httptest.NewRecorder()

	h := &Handler{
		KubeClient: &KubeClient{}, // not used in this test
		Metrics:    metrics.InitPrometheus(),
	}

	h.SubmitHandler(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestSubmitHandler_InvalidNamespace(t *testing.T) {
	body := strings.NewReader("type=Storage&name=mystorage&username=missing&region=US")
	req := httptest.NewRequest(http.MethodPost, "/submit", body)
	_ = req.ParseForm()
	req.Form.Set("type", "Storage")
	req.Form.Set("name", "mystorage")
	req.Form.Set("username", "missing")
	req.Form.Set("region", "US")
	rr := httptest.NewRecorder()

	h := &Handler{
		KubeClient: NewKubernetesClient(),
		Metrics:    metrics.InitPrometheus(),
	}

	h.SubmitHandler(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Equal(t, 1, int(testutil.ToFloat64(h.Metrics.ClaimsFailed.WithLabelValues("US", "missing"))))
}

// We can mock Kube.DynamicClient with github.com/stretchr/testify/mock later for deeper tests.
