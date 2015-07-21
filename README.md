# render #

This project is written by golang. Easily rendering serialized JSON, XML, and HTML template responses use http.ResponseWriter. It's migration from 'github.com/martini-contrib/render'

## Usage ##

```golang
package main

import (
	"html/template"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"openeasy.net/openeasy/render"
)

func main() {
	// 初始化Render
	render.Renderer(render.Options{
		Directory:  "templates",               // Specify what path to load the templates from.
		Layout:     "layout",                  // Specify a layout template. Layouts can call {{ yield }} to render the current template.
		Extensions: []string{".tmpl"},         // Specify extensions to load for templates.
		Delims:     render.Delims{"{{", "}}"}, // Sets delimiters to the specified strings.
		Charset:    "UTF-8",                   // Sets encoding for json and html content-types. Default is "UTF-8".
		// IndentJSON: true,                        // Output human readable JSON
		// IndentXML:  true,                        // Output human readable XML
		// HTMLContentType: "application/xhtml+xml",   // Output XHTML content type instead of default "text/html"
	})

	r := gin.Default()
	// 添加路由
	r.GET("/", func(ctx *gin.Context) {
		// 设置参数
		d := map[string]template.HTML{
			"Title": "登陆",
		}
		// 获取实例
		r := render.GetInst()
		// 渲染HTML
		r.HTML(ctx.Writer, 200, "index", d)
	})
	// 设置http服务
	s := &http.Server{
		Addr:           ":8080",
		Handler:        r,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	s.ListenAndServe()
}
```
The following templates:
```html
<!-- templates/layout.tmpl -->
<!doctype html>
<html lang="zh">
<head>
<title>{{.Title}}</title>
<meta http-equiv="Content-Type" content="text/html; charset=UTF-8"/>
<link rel="shortcut icon" type="image/x-icon" href="/favicon.ico" />
</head>
<body>
<!-- Render the current template here -->
{{ yield }}
</body>
</html>
```

```html
<!-- templates/index.tmpl -->
<h1>{{.Name}}</h1>
<ul>
	<li>{{template "base/header"}}</li>
</ul>
```

```html
<!-- templates/base/header.tmpl -->
<h2>我是头</h2>
```

## Authors ##
[Zhang Chaoren](https://openeasy.net/cr "Zhang Chaoren")