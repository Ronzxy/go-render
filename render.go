/* Copyright 2018 sky<skygangsta@hotmail.com>. All rights reserved.
 *
 * Licensed under the Apache License, version 2.0 (the "License").
 * You may not use this work except in compliance with the License, which is
 * available at www.apache.org/licenses/LICENSE-2.0
 *
 * This software is distributed on an "AS IS" basis, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied, as more fully set forth in the License.
 *
 * See the NOTICE file distributed with this work for information regarding copyright ownership.
 */

package render

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"github.com/skygangsta/go-helper"
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
	render  *template.Template
	buffer  *helper.BufferPool
	options Options
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

// Delimiter represents a set of Left and Right delimiters for HTML template rendering
type Delimiter struct {
	// Left delimiter, defaults to {{
	Left string
	// Right delimiter, defaults to }}
	Right string
}

// Options is a struct for specifying configuration options for the render.Render middleware
type Options struct {
	// Directory to load templates. Default is "templates"
	Directory string
	// Layout template name. Will not render a layout if "". Defaults to "".
	Layout string
	// Extensions to parse template files from. Defaults to [".tmpl"]
	Extensions []string
	// Funcs is a slice of FuncMap to apply to the template upon compilation. This is useful for helper functions. Defaults to [].
	FuncMap template.FuncMap
	// Delimiter sets the action delimiters to the specified strings in the Delimiter struct.
	Delimiter Delimiter
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
	BufferPool      int
}

// HTMLOptions is a struct for overriding some rendering Options for specific HTML call
type HTMLOptions struct {
	// Layout template name. Overrides Options.Layout.
	Layout string
}

// Render is a external rendering. An single variadic render.Options struct can be optionally provided to configure HTML
// rendering. The default directory for templates is "templates" and the default file extension is ".tmpl".
func Render(o Options) {
	options = prepareOptions(o)
	render = createTemplate(options)
	buffer = helper.NewBufferPool(options.BufferPool)
}

func prepareCharset(charset string) string {
	if len(charset) != 0 {
		return "; charset=" + charset
	}

	return "; charset=" + defaultCharset
}

func prepareOptions(options Options) Options {
	// Defaults
	if len(options.Directory) == 0 {
		options.Directory = "templates"
	}
	if len(options.Extensions) == 0 {
		options.Extensions = []string{".tmpl"}
	}
	if len(options.HTMLContentType) == 0 {
		options.HTMLContentType = ContentHTML
	}

	if options.BufferPool == 0 {
		options.BufferPool = 128
	}

	return options
}

func createTemplate(options Options) *template.Template {
	dir := options.Directory

	t := template.New(dir)
	t.Delims(options.Delimiter.Left, options.Delimiter.Right)

	// check template file error
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		relativePath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		ext := getExt(relativePath)

		for _, extension := range options.Extensions {
			if ext == extension {

				buf, err := ioutil.ReadFile(path)
				if err != nil {
					panic(err)
				}

				name := relativePath[0 : len(relativePath)-len(ext)]
				tmpl := t.New(filepath.ToSlash(name))

				tmpl.Funcs(options.FuncMap)

				// Bomb out if parse fails. When the server starts.
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

func JSON(w http.ResponseWriter, status int, v interface{}) {
	var result []byte
	var err error
	if options.IndentJSON {
		result, err = json.MarshalIndent(v, "", "  ")
	} else {
		result, err = json.Marshal(v)
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// json rendered fine, write out the result
	w.Header().Set(ContentType, ContentJSON+prepareCharset(options.Charset))
	w.WriteHeader(status)
	if len(options.PrefixJSON) > 0 {
		w.Write(options.PrefixJSON)
	}
	w.Write(result)
}

func HTML(w http.ResponseWriter, status int, name string, binding interface{}, htmlOptions ...HTMLOptions) {
	option := prepareHTMLOptions(htmlOptions)
	// assign a layout if there is one
	if len(option.Layout) > 0 {
		addYield(name, binding)
		name = option.Layout
	}

	buf, err := execute(name, binding)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// template rendered fine, write out the result
	w.Header().Set(ContentType, options.HTMLContentType+prepareCharset(options.Charset))
	w.WriteHeader(status)
	io.Copy(w, buf)
	// Set buffer in BufferPool
	buffer.Set(buf)
}

func XML(w http.ResponseWriter, status int, v interface{}) {
	var result []byte
	var err error
	if options.IndentXML {
		result, err = xml.MarshalIndent(v, "", "  ")
	} else {
		result, err = xml.Marshal(v)
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// XML rendered fine, write out the result
	w.Header().Set(ContentType, ContentXML+prepareCharset(options.Charset))
	w.WriteHeader(status)
	if len(options.PrefixXML) > 0 {
		w.Write(options.PrefixXML)
	}
	w.Write(result)
}

func Data(w http.ResponseWriter, status int, v []byte) {
	if w.Header().Get(ContentType) == "" {
		w.Header().Set(ContentType, ContentBinary)
	}
	w.WriteHeader(status)
	w.Write(v)
}

func Text(w http.ResponseWriter, status int, v string) {
	if w.Header().Get(ContentType) == "" {
		w.Header().Set(ContentType, ContentText+prepareCharset(options.Charset))
	}
	w.WriteHeader(status)
	w.Write([]byte(v))
}

// Error writes the given HTTP status to the current ResponseWriter
func Error(w http.ResponseWriter, status int, v []byte) {
	w.WriteHeader(status)
	w.Write(v)

}

func Status(w http.ResponseWriter, status int) {
	w.WriteHeader(status)
}

func Redirect(w http.ResponseWriter, r *http.Request, status int, location string) {
	code := http.StatusFound
	if status != 0 {
		code = status
	}

	http.Redirect(w, r, location, code)
}

func Template() *template.Template {
	return render
}

func execute(name string, binding interface{}) (*bytes.Buffer, error) {
	// Get buffer in BufferPool
	buf := buffer.Get()

	return buf, render.ExecuteTemplate(buf, name, binding)
}

func addYield(name string, binding interface{}) {
	funcs := template.FuncMap{
		"yield": func() (template.HTML, error) {
			buf, err := execute(name, binding)
			// return safe html here since we are rendering our own template
			return template.HTML(buf.String()), err
		},
		"current": func() (string, error) {
			return name, nil
		},
	}
	render.Funcs(funcs)
}

func prepareHTMLOptions(htmlOptions []HTMLOptions) HTMLOptions {
	if len(htmlOptions) > 0 {
		return htmlOptions[0]
	}

	return HTMLOptions{
		Layout: options.Layout,
	}
}
