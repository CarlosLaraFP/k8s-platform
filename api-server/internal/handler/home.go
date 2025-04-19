package handler

import (
	"api-server/internal/metrics"
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

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

var (
	scheme    = runtime.NewScheme()
	clientset *kubernetes.Clientset
	client    *dynamic.DynamicClient
	gvr       = schema.GroupVersionResource{
		Group:    "platform.example.org",
		Version:  "v1alpha1",
		Resource: "storage",
	}
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
}

func NewKubernetesClient() {
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("Error loading kubeconfig: %v", err)
	}

	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating Kubernetes dynamic clientset: %v", err)
	}
	clientset = cs

	c, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating Kubernetes dynamic client: %v", err)
	}
	client = c
}

var templates = template.Must(template.ParseFiles(
	"web/templates/view.html",
	"web/templates/edit.html",
	"web/templates/index.html",
	"web/templates/list.html",
))
var validPath = regexp.MustCompile("^/(submit|edit|view)/([a-zA-Z0-9]+)$")
var validDNSName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

type Claim struct {
	Name      string
	Region    string
	Namespace string
}

// apply uses client-go to create a Crossplane Claim
func (c *Claim) apply(ctx context.Context) error {
	claim := &unstructured.Unstructured{}
	// sets the metadata on the object being created
	claim.SetAPIVersion("platform.example.org/v1alpha1")
	claim.SetKind("Storage")
	claim.SetName(c.Name)
	claim.SetNamespace(c.Namespace)
	err := unstructured.SetNestedField(claim.Object, c.Region, "spec", "location")
	if err != nil {
		return fmt.Errorf("error setting spec.location: %w", err)
	}

	discoveryClient, err := clientset.Discovery().ServerResourcesForGroupVersion("platform.example.org/v1alpha1")
	if err != nil {
		return fmt.Errorf("❌ Discovery error: %w", err)
	}
	for _, res := range discoveryClient.APIResources {
		log.Printf("✅ Discovered: name=%s kind=%s namespaced=%v\n", res.Name, res.Kind, res.Namespaced)
	}

	// sets the target API endpoint for the request: POST /apis/platform.example.org/v1alpha1/namespaces/dev-user/storage
	if _, err := client.Resource(gvr).Namespace(c.Namespace).Create(ctx, claim, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("error creating the claim: %w", err)
	}
	return nil
}

func loadClaim(name string) (*Claim, error) {
	filename := name + ".txt"
	region, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return &Claim{Name: name, Region: string(region)}, nil
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
	err := templates.ExecuteTemplate(w, tmpl+".html", c)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func ViewHandler(w http.ResponseWriter, r *http.Request, claim string) {
	c, err := loadClaim(claim)
	if err != nil {
		http.Redirect(w, r, "/edit/"+claim, http.StatusFound)
		return
	}
	renderTemplate(w, "view", c)
}

func EditHandler(w http.ResponseWriter, r *http.Request, name string) {
	p, err := loadClaim(name)
	if err != nil {
		p = &Claim{Name: name, Region: "US"}
	}
	renderTemplate(w, "edit", p)
}

func SubmitHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	// Validating the name to match Kubernetes DNS subdomain rules
	name := strings.ToLower(r.FormValue("name"))
	if !validDNSName.MatchString(name) || len(name) > 63 {
		http.Error(w, "Invalid claim name: must match [a-z0-9]([-a-z0-9]*[a-z0-9])? and < 64 characters", http.StatusBadRequest)
		return
	}

	c := &Claim{
		Name:      name,
		Region:    r.FormValue("region"),
		Namespace: r.FormValue("username"),
	}

	defer func() {
		metrics.ClaimLatency.
			WithLabelValues(c.Region, c.Namespace).
			Observe(time.Since(start).Seconds())
	}()

	if err := c.apply(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		metrics.ClaimsFailed.WithLabelValues(c.Region, c.Namespace).Inc()
		return
	}

	metrics.ClaimsSubmitted.WithLabelValues(c.Region, c.Namespace).Inc()

	http.Redirect(w, r, "/view/"+name, http.StatusFound)
}

type ClaimView struct {
	Name      string
	Location  string
	Namespace string
	Status    string
}

func GetClaims(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("ns")
	if ns == "" {
		ns = "dev-user"
	}
	list, err := client.
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
	if err := templates.ExecuteTemplate(w, "list.html", cv); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
