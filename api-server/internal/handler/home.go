package internal

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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

var templates = template.Must(template.ParseFiles("web/templates/view.html", "web/templates/edit.html", "web/templates/index.html"))
var validPath = regexp.MustCompile("^/(submit|edit|view)/([a-zA-Z0-9]+)$")

type Claim struct {
	Name   string
	Region string
}

// apply uses client-go to create a Crossplane Claim
func (c *Claim) apply(ctx context.Context) error {
	claim := &unstructured.Unstructured{}
	// sets the metadata on the object being created
	claim.SetAPIVersion("platform.example.org/v1alpha1")
	claim.SetKind("Storage")
	claim.SetName(c.Name)
	claim.SetNamespace("dev-user")
	err := unstructured.SetNestedField(claim.Object, c.Region, "spec", "location")
	if err != nil {
		return fmt.Errorf("error setting spec.location: %w", err)
	}
	gvr := schema.GroupVersionResource{
		Group:    "platform.example.org",
		Version:  "v1alpha1",
		Resource: "storage",
	}

	discoveryClient, err := clientset.Discovery().ServerResourcesForGroupVersion("platform.example.org/v1alpha1")
	if err != nil {
		return fmt.Errorf("❌ Discovery error: %w", err)
	}
	for _, res := range discoveryClient.APIResources {
		log.Printf("✅ Discovered: name=%s kind=%s namespaced=%v\n", res.Name, res.Kind, res.Namespaced)
	}

	// sets the target API endpoint for the request: POST /apis/platform.example.org/v1alpha1/namespaces/dev-user/storage
	_, err = client.Resource(gvr).Namespace("dev-user").Create(ctx, claim, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("error creating the claim: %w", err)
	}
	return nil
}

func (c *Claim) save() error {
	filename := c.Name + ".txt"
	return os.WriteFile(filename, []byte(c.Region), 0600)
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
		p = &Claim{Name: name}
	}
	renderTemplate(w, "edit", p)
}

func SubmitHandler(w http.ResponseWriter, r *http.Request, name string) {
	ctx := r.Context()
	select {
	case <-ctx.Done():
		return
	default:
	}

	c := &Claim{
		Name:   strings.ToLower(name),
		Region: r.FormValue("region"),
	}
	err := c.apply(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/view/"+name, http.StatusFound)
}
