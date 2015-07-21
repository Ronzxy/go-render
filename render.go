// This file is copy from 'github.com/martini-contrib/render' which is under
// The MIT License (MIT)

package render

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	ContentType    = "Content-Type"
	ContentLength  = "Content-Length"
	ContentBinary  = "application/octet-stream"
	ContentText    = "text/plain"
	ContentJSON    = "application/json"
	ContentHTML    = "text/html"
	ContentXHTML   = "application/xhtml+xml"
	ContentXML     = "text/xml"
	defaultCharset = "UTF-8"
)

var (
	render *renderer
	buffer *BufferPool
)

// Included helper functions for use when rendering html
var helperFuncs = template.FuncMap{
	"yield": func() (string, error) {
		return "", fmt.Errorf("yield called with no layout defined")
	},
	"current": func() (string, error) {
		return "", nil
	},
}

// Delims represents a set of Left and Right delimiters for HTML template rendering
type Delims struct {
	// Left delimiter, defaults to {{
	Left string
	// Right delimiter, defaults to }}
	Right string
}

// Options is a struct for specifying configuration options for the render.Renderer middleware
type Options struct {
	// Directory to load templates. Default is "templates"
	Directory string
	// Layout template name. Will not render a layout if "". Defaults to "".
	Layout string
	// Extensions to parse template files from. Defaults to [".tmpl"]
	Extensions []string
	// Funcs is a slice of FuncMaps to apply to the template upon compilation. This is useful for helper functions. Defaults to [].
	Funcs []template.FuncMap
	// Delims sets the action delimiters to the specified strings in the Delims struct.
	Delims Delims
	// Appends the given charset to the Content-Type header. Default is "UTF-8".
	Charset string
	// Outputs human readable JSON
	IndentJSON bool
	// Outputs human readable XML
	IndentXML bool
	// Prefixes the JSON output with the given bytes.
	PrefixJSON []byte
	// Prefixes the XML output with the given bytes.
	PrefixXML []byte
	// Allows changing of output to XHTML instead of HTML. Default is "text/html"
	HTMLContentType string
}

// HTMLOptions is a struct for overriding some rendering Options for specific HTML call
type HTMLOptions struct {
	// Layout template name. Overrides Options.Layout.
	Layout string
}

// Renderer is a external rendering. An single variadic render.Options struct can be optionally provided to configure HTML
// rendering. The default directory for templates is "templates" and the default file extension is ".tmpl".
func Renderer(options ...Options) {
	opt := prepareOptions(options)
	cs := prepareCharset(opt.Charset)
	t := compile(opt)
	buffer = NewBufferPool(64)

	render = &renderer{t, opt, cs}
}

// GetInst is get renderer instance.
func GetInst() *renderer {
	return render
}

func prepareCharset(charset string) string {
	if len(charset) != 0 {
		return "; charset=" + charset
	}

	return "; charset=" + defaultCharset
}

func prepareOptions(options []Options) Options {
	var opt Options
	if len(options) > 0 {
		opt = options[0]
	}

	// Defaults
	if len(opt.Directory) == 0 {
		opt.Directory = "templates"
	}
	if len(opt.Extensions) == 0 {
		opt.Extensions = []string{".tmpl"}
	}
	if len(opt.HTMLContentType) == 0 {
		opt.HTMLContentType = ContentHTML
	}

	return opt
}

func compile(options Options) *template.Template {
	dir := options.Directory
	t := template.New(dir)
	t.Delims(options.Delims.Left, options.Delims.Right)
	// parse an initial template in case we don't have any
	template.Must(t.Parse("TMPL"))

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		r, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		ext := getExt(r)

		for _, extension := range options.Extensions {
			if ext == extension {

				buf, err := ioutil.ReadFile(path)
				if err != nil {
					panic(err)
				}

				name := (r[0 : len(r)-len(ext)])
				tmpl := t.New(filepath.ToSlash(name))

				// add our funcmaps
				for _, funcs := range options.Funcs {
					tmpl.Funcs(funcs)
				}

				// Bomb out if parse fails. We don't want any silent server starts.
				template.Must(tmpl.Funcs(helperFuncs).Parse(string(buf)))
				break
			}
		}

		return nil
	})

	return t
}

func getExt(s string) string {
	if strings.Index(s, ".") == -1 {
		return ""
	}
	return "." + strings.Join(strings.Split(s, ".")[1:], ".")
}

type renderer struct {
	// http.ResponseWriter
	// req             *http.Request
	t               *template.Template
	opt             Options
	compiledCharset string
}

func (r *renderer) JSON(w http.ResponseWriter, status int, v interface{}) {
	var result []byte
	var err error
	if r.opt.IndentJSON {
		result, err = json.MarshalIndent(v, "", "  ")
	} else {
		result, err = json.Marshal(v)
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// json rendered fine, write out the result
	w.Header().Set(ContentType, ContentJSON+r.compiledCharset)
	w.WriteHeader(status)
	if len(r.opt.PrefixJSON) > 0 {
		w.Write(r.opt.PrefixJSON)
	}
	w.Write(result)
}

func (r *renderer) HTML(w http.ResponseWriter, status int, name string, binding interface{}, htmlOpt ...HTMLOptions) {
	opt := r.prepareHTMLOptions(htmlOpt)
	// assign a layout if there is one
	if len(opt.Layout) > 0 {
		r.addYield(name, binding)
		name = opt.Layout
	}

	buf, err := r.execute(name, binding)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// template rendered fine, write out the result
	w.Header().Set(ContentType, r.opt.HTMLContentType+r.compiledCharset)
	w.WriteHeader(status)
	io.Copy(w, buf)
	// Put buffer in BufferPool
	buffer.Put(buf)
}

func (r *renderer) XML(w http.ResponseWriter, status int, v interface{}) {
	var result []byte
	var err error
	if r.opt.IndentXML {
		result, err = xml.MarshalIndent(v, "", "  ")
	} else {
		result, err = xml.Marshal(v)
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// XML rendered fine, write out the result
	w.Header().Set(ContentType, ContentXML+r.compiledCharset)
	w.WriteHeader(status)
	if len(r.opt.PrefixXML) > 0 {
		w.Write(r.opt.PrefixXML)
	}
	w.Write(result)
}

func (r *renderer) Data(w http.ResponseWriter, status int, v []byte) {
	if w.Header().Get(ContentType) == "" {
		w.Header().Set(ContentType, ContentBinary)
	}
	w.WriteHeader(status)
	w.Write(v)
}

func (r *renderer) Text(w http.ResponseWriter, status int, v string) {
	if w.Header().Get(ContentType) == "" {
		w.Header().Set(ContentType, ContentText+r.compiledCharset)
	}
	w.WriteHeader(status)
	w.Write([]byte(v))
}

// Error writes the given HTTP status to the current ResponseWriter
func (r *renderer) Error(w http.ResponseWriter, status int) {
	w.WriteHeader(status)
}

func (r *renderer) Status(w http.ResponseWriter, status int) {
	w.WriteHeader(status)
}

func (r *renderer) Redirect(w http.ResponseWriter, req *http.Request, location string, status ...int) {
	code := http.StatusFound
	if len(status) == 1 {
		code = status[0]
	}

	http.Redirect(w, req, location, code)
}

func (r *renderer) Template() *template.Template {
	return r.t
}

func (r *renderer) execute(name string, binding interface{}) (*bytes.Buffer, error) {
	// Get buffer in BufferPool
	buf := buffer.Get()

	return buf, r.t.ExecuteTemplate(buf, name, binding)
}

func (r *renderer) addYield(name string, binding interface{}) {
	funcs := template.FuncMap{
		"yield": func() (template.HTML, error) {
			buf, err := r.execute(name, binding)
			// return safe html here since we are rendering our own template
			return template.HTML(buf.String()), err
		},
		"current": func() (string, error) {
			return name, nil
		},
	}
	r.t.Funcs(funcs)
}

func (r *renderer) prepareHTMLOptions(htmlOpt []HTMLOptions) HTMLOptions {
	if len(htmlOpt) > 0 {
		return htmlOpt[0]
	}

	return HTMLOptions{
		Layout: r.opt.Layout,
	}
}
