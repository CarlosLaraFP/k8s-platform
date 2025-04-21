package handler

import (
	"api-server/internal/metrics"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type FakeClaimer struct {
	ShouldFail bool
	GVRs       map[Resource]schema.GroupVersion
}

func (f *FakeClaimer) CreateClaim(ctx context.Context, c *Claim) error {
	if f.ShouldFail {
		return fmt.Errorf("simulated failure")
	}
	return nil
}

func (f *FakeClaimer) GetClaims(w http.ResponseWriter, r *http.Request) {
}

func (f *FakeClaimer) VerifyGVR(r Resource) *schema.GroupVersion {
	gv, ok := f.GVRs[r]
	if !ok {
		log.Printf("❌ Resource *%v* not found in supported GVRs", r)
		return nil
	}
	return &gv
}

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
		Claimer: new(FakeClaimer),     // not used in this test
		Metrics: new(metrics.Metrics), // not used in this test
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
		Claimer: new(FakeClaimer),     // not used in this test
		Metrics: new(metrics.Metrics), // not used in this test
	}

	h.SubmitHandler(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestSubmitHandler_InvalidNamespace(t *testing.T) {
	//body := strings.NewReader("type=Storage&name=mystorage&username=missing&region=US")
	req := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(("")))
	_ = req.ParseForm()
	req.Form.Set("type", "Storage")
	req.Form.Set("name", "mystorage")
	req.Form.Set("username", "missing")
	req.Form.Set("region", "US")
	rr := httptest.NewRecorder()

	h := &Handler{
		Claimer: &FakeClaimer{
			ShouldFail: true,
			GVRs: map[Resource]schema.GroupVersion{
				"storage": {
					Group:   "platform.example.org",
					Version: "v1alpha1",
				},
			},
		},
		Metrics: metrics.InitPrometheus(),
	}

	h.SubmitHandler(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Equal(t, 1, int(testutil.ToFloat64(h.Metrics.ClaimsFailed.WithLabelValues("US", "missing"))))
}
