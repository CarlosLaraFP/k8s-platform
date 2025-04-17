package internal

import (
	"html/template"
	"net/http"
	"os"
	"regexp"
)

var templates = template.Must(template.ParseFiles("web/templates/view.html", "web/templates/edit.html", "web/templates/index.html"))
var validPath = regexp.MustCompile("^/(submit|edit|view)/([a-zA-Z0-9]+)$")

type Claim struct {
	Name     string
	Metadata []byte
}

func (c *Claim) save() error {
	filename := c.Name + ".txt"
	return os.WriteFile(filename, c.Metadata, 0600)
}

func loadClaim(name string) (*Claim, error) {
	filename := name + ".txt"
	metadata, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return &Claim{Name: name, Metadata: metadata}, nil
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
	p, err := loadClaim(claim)
	if err != nil {
		http.Redirect(w, r, "/edit/"+claim, http.StatusFound)
		return
	}
	renderTemplate(w, "view", p)
}

func EditHandler(w http.ResponseWriter, r *http.Request, name string) {
	p, err := loadClaim(name)
	if err != nil {
		p = &Claim{Name: name}
	}
	renderTemplate(w, "edit", p)
}

func SubmitHandler(w http.ResponseWriter, r *http.Request, name string) {
	metadata := r.FormValue("body")
	p := &Claim{Name: name, Metadata: []byte(metadata)}
	err := p.save()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/view/"+name, http.StatusFound)
}
