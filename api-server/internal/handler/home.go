package handler

import (
	"api-server/internal/metrics"
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type Resource string

type KubeClient struct {
	DynamicClient dynamic.Interface
	Clientset     kubernetes.Interface
	Scheme        *runtime.Scheme
	GVRs          map[Resource]schema.GroupVersion
}

func NewKubernetesClient() *KubeClient {
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("Error loading kubeconfig: %v", err)
	}

	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating Kubernetes dynamic clientset: %v", err)
	}

	c, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating Kubernetes dynamic client: %v", err)
	}

	// scheme is only used when decoding Kubernetes runtime objects into Go types (i.e. clientset.AppsV1().Deployments(...))
	// support serialization and deserialization of Kubernetes objects
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	return &KubeClient{
		DynamicClient: c,
		Clientset:     cs,
		Scheme:        runtime.NewScheme(),
		GVRs: map[Resource]schema.GroupVersion{
			"storage": { // K8s API
				Group:   "platform.example.org",
				Version: "v1alpha1",
			},
		},
	}
}

type Handler struct {
	KubeClient *KubeClient
	Metrics    *metrics.Metrics
}

func (h *Handler) SubmitHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	// When the form is submitted in the browser via HTML,
	// the browser encodes the fields into a body like "name=foo&username=bar&type=storage"
	// and sets Content-Type: application/x-www-form-urlencoded
	t := strings.ToLower(r.FormValue("type"))
	ns := r.FormValue("username")
	// Validating the name to match Kubernetes DNS subdomain rules
	name := strings.ToLower(r.FormValue("name"))
	if !validDNSName.MatchString(name) || len(name) > 63 {
		http.Error(w, "Invalid claim name: must match [a-z0-9]([-a-z0-9]*[a-z0-9])? and < 64 characters", http.StatusBadRequest)
		return
	}

	gv, ok := h.KubeClient.GVRs[Resource(t)]
	if !ok {
		http.Error(w, fmt.Sprintf("❌ Resource *%v* not found in supported GVRs", t), http.StatusBadRequest)
	}

	c := &Claim{
		Name: name,
		GVR: schema.GroupVersionResource{
			Group:    gv.Group,
			Version:  gv.Version,
			Resource: t,
		},
		Region:    r.FormValue("region"),
		Namespace: ns,
	}

	defer func() {
		h.Metrics.ClaimLatency.
			WithLabelValues(c.Region, c.Namespace).
			Observe(time.Since(start).Seconds())
	}()

	if err := c.apply(r.Context(), h.KubeClient); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		h.Metrics.ClaimsFailed.WithLabelValues(c.Region, c.Namespace).Inc()
		return
	}

	h.Metrics.ClaimsSubmitted.WithLabelValues(c.Region, c.Namespace).Inc()

	http.Redirect(w, r, fmt.Sprintf("/claims?ns=%s", ns), http.StatusFound)
}

type ClaimView struct {
	Name      string
	Location  string
	Namespace string
	Status    string
}

type Claim struct {
	Name      string
	GVR       schema.GroupVersionResource
	Region    string
	Namespace string
}

// apply uses client-go to create a Crossplane Claim based on user request
func (c *Claim) apply(ctx context.Context, kube *KubeClient) error {
	claim := &unstructured.Unstructured{}
	// sets the metadata on the object being created
	claim.SetAPIVersion(fmt.Sprintf("%s/%s", c.GVR.Group, c.GVR.Version))
	claim.SetKind(cases.Title(language.English).String(c.GVR.Resource))
	claim.SetName(c.Name)
	claim.SetNamespace(c.Namespace)
	err := unstructured.SetNestedField(claim.Object, c.Region, "spec", "location")
	if err != nil {
		return fmt.Errorf("error setting spec.location: %w", err)
	}

	discoveryClient, err := kube.Clientset.Discovery().ServerResourcesForGroupVersion(fmt.Sprintf("%s/%s", c.GVR.Group, c.GVR.Version))
	if err != nil {
		return fmt.Errorf("❌ Discovery error: %w", err)
	}
	for _, res := range discoveryClient.APIResources {
		log.Printf("✅ Discovered: name=%s kind=%s namespaced=%v\n", res.Name, res.Kind, res.Namespaced)
	}

	// sets the target API endpoint for the request: POST /apis/platform.example.org/v1alpha1/namespaces/dev-user/storage
	if _, err := kube.DynamicClient.Resource(c.GVR).Namespace(c.Namespace).Create(ctx, claim, metav1.CreateOptions{}); err != nil {
		log.Printf("❌ Failed to create claim: %v", err)
		return fmt.Errorf("error creating the claim: %w", err)
	}
	return nil
}

func (h *Handler) GetClaims(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("ns")
	// TODO: Check if namespace exists
	if ns == "" {
		ns = "dev-user"
	}
	rs := r.URL.Query().Get("type")
	gv, ok := h.KubeClient.GVRs[Resource(rs)]
	if !ok {
		http.Error(w, fmt.Sprintf("❌ Resource *%v* not found in supported GVRs", rs), http.StatusInternalServerError)
	}
	gvr := schema.GroupVersionResource{
		Group:    gv.Group,
		Version:  gv.Version,
		Resource: rs,
	}
	list, err := h.KubeClient.
		DynamicClient.
		Resource(gvr).
		Namespace(ns).
		List(r.Context(), metav1.ListOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	cv := make([]ClaimView, 0)

	for _, r := range list.Items {
		name := r.GetName()
		location, _, _ := unstructured.NestedString(r.Object, "spec", "location")
		conditions, _, _ := unstructured.NestedSlice(r.Object, "status", "conditions")
		status := "Unknown"

		for _, cond := range conditions {
			if condMap, ok := cond.(map[string]any); ok {
				if condMap["type"] == "Ready" {
					if _, ok := condMap["status"].(string); ok {
						status = "Ready"
						break
					}
				}
			}
		}

		cv = append(cv, ClaimView{
			Name:      name,
			Location:  location,
			Namespace: ns,
			Status:    status,
		})
	}
	if err := loadTemplates().ExecuteTemplate(w, "list.html", cv); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func loadTemplates() *template.Template {
	return template.Must(template.ParseFiles(
		"web/templates/view.html",
		"web/templates/edit.html",
		"web/templates/index.html",
		"web/templates/list.html",
	))
}

var validPath = regexp.MustCompile("^/(submit|edit|view)/([a-zA-Z0-9]+)$")
var validDNSName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

func ViewHandler(w http.ResponseWriter, r *http.Request, claim string) {
	// TODO: Pull from ETCD
	c := &Claim{Name: claim, Region: "US"}
	renderTemplate(w, "view", c)
}

func EditHandler(w http.ResponseWriter, r *http.Request, name string) {
	c := &Claim{Name: name, Region: "US"}
	renderTemplate(w, "edit", c)
}

func MakeHandler(fn func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := validPath.FindStringSubmatch(r.URL.Path)
		if m == nil {
			http.NotFound(w, r)
			return
		}
		fn(w, r, m[2])
	}
}

func renderTemplate(w http.ResponseWriter, tmpl string, c *Claim) {
	err := loadTemplates().ExecuteTemplate(w, tmpl+".html", c)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
