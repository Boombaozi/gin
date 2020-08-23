package gin

import (
	"encoding/json"
	"encoding/xml"
	"html/template"
	"log"
	"math"
	"net/http"
	"path"

	"github.com/julienschmidt/httprouter"
)

//gin 提供了 路由分组，及中间件 两个特性，而 中间件是 挂载在路由分组上的，也就是 可以灵活的 将 路由组 与中间件进行组合

//BUG gin.go 总结
//此文件定义了 gin核心的一些结构题 及核心的 方法
//1.定义了 中间件的形式，即 传入一个上下文的 func
//2.定义了 Context ， 包含了  req, 输出流，handler[] ,engine , 及中间件执行位置的index
//3.engine,
//4. routergroup 路由分组

//gin 启动流程
//1.  gin.new() 初始化一个空的 engine 结构体
//2.  使用 gin.Default()  会 调用 gin.use()  放入两个中间件(错误捕获，及 日志) 到当前 路由组
//3。 r.Get 等方法，将 路由 及 handler 添加到 router 中, 此 handler内容为 创建上下文及执行下一个 handler

//gin 请求处理流程
//1. router 进行路由匹配，调用 func
//2. 此func 创建上下文 并调用第一个中间件

const (
	AbortIndex = math.MaxInt8 / 2
)

type (

	//中间件函数 类型， 需传入上下文类型的指针
	HandlerFunc func(*Context)

	//将 string: interface{} 定义为 H
	H map[string]interface{}

	//定义错误信息
	// Used internally to collect a error ocurred during a http request.
	ErrorMsg struct {
		Message string      `json:"msg"`
		Meta    interface{} `json:"meta"`
	}

	// Context is the most important part of gin. It allows us to pass variables between middleware,
	// manage the flow, validate the JSON of a request and render a JSON response for example.
	Context struct {
		Req *http.Request

		Writer http.ResponseWriter
		Keys   map[string]interface{}
		//TODO : 收集多个错误信息的机制？？
		//此处会收集所有的中间件 放入的错误
		Errors []ErrorMsg
		Params httprouter.Params
		//中间件
		handlers []HandlerFunc
		engine   *Engine
		index    int8
	}

	// Used internally to configure router, a RouterGroup is associated with a prefix
	// and an array of handlers (middlewares)
	RouterGroup struct {
		//属于某个 路由分组的 中间件
		Handlers []HandlerFunc
		// 前缀
		prefix string
		//父路由
		parent *RouterGroup
		engine *Engine
	}

	// Represents the web framework, it wrappers the blazing fast httprouter multiplexer and a list of global middlewares.
	Engine struct {
		*RouterGroup
		//TODO 确认handers404的作用
		//
		handlers404   []HandlerFunc
		router        *httprouter.Router
		HTMLTemplates *template.Template
	}
)

// Returns a new blank Engine instance without any middleware attached.
// The most basic configuration
func New() *Engine {
	engine := &Engine{}
	engine.RouterGroup = &RouterGroup{nil, "", nil, engine}
	engine.router = httprouter.New()
	//指定router的 404 处理方法
	engine.router.NotFound = engine.handle404
	return engine
}

// Returns a Engine instance with the Logger and Recovery already attached.
func Default() *Engine {
	engine := New()
	engine.Use(Recovery(), Logger())
	return engine
}

func (engine *Engine) LoadHTMLTemplates(pattern string) {
	engine.HTMLTemplates = template.Must(template.ParseGlob(pattern))
}

//用户自己指定的 404 handers
// Adds handlers for NotFound. It return a 404 code by default.
func (engine *Engine) NotFound404(handlers ...HandlerFunc) {
	engine.handlers404 = handlers
}

// 404 处理方法，传给router
//如果用户未定义 404 中间件，则返回默认的404错误。 如果有则 执行全部中间件
func (engine *Engine) handle404(w http.ResponseWriter, req *http.Request) {

	//使分组路由 与 用户指定的404 handers放入一起， 放在上下文中
	handlers := engine.combineHandlers(engine.handlers404)

	//创建上下文
	c := engine.createContext(w, req, nil, handlers)

	if engine.handlers404 == nil {
		http.NotFound(c.Writer, c.Req)
	} else {
		c.Writer.WriteHeader(404)
	}
	c.Next()
}

// ServeFiles serves files from the given file system root.
// The path must end with "/*filepath", files are then served from the local
// path /defined/root/dir/*filepath.
// For example if root is "/etc" and *filepath is "passwd", the local file
// "/etc/passwd" would be served.
// Internally a http.FileServer is used, therefore http.NotFound is used instead
// of the Router's NotFound handler.
// To use the operating system's file system implementation,
// use http.Dir:
//     router.ServeFiles("/src/*filepath", http.Dir("/var/www"))
func (engine *Engine) ServeFiles(path string, root http.FileSystem) {
	engine.router.ServeFiles(path, root)
}

// ServeHTTP makes the router implement the http.Handler interface.
func (engine *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	engine.router.ServeHTTP(w, req)
}

func (engine *Engine) Run(addr string) {
	//TODO : 确认此处传入engine的作用
	http.ListenAndServe(addr, engine)
}

/************************************/
/********** ROUTES GROUPING *********/
/************************************/

// 创建上下文环境，用于中间件及 业务代码
func (group *RouterGroup) createContext(w http.ResponseWriter, req *http.Request, params httprouter.Params, handlers []HandlerFunc) *Context {
	return &Context{
		Writer:   w,
		Req:      req,
		index:    -1,
		engine:   group.engine,
		Params:   params,
		handlers: handlers,
	}
}

// Adds middlewares to the group, see example code in github.
func (group *RouterGroup) Use(middlewares ...HandlerFunc) {
	group.Handlers = append(group.Handlers, middlewares...)
}

// Greates a new router group. You should create add all the routes that share that have common middlwares or same path prefix.
// For example, all the routes that use a common middlware for authorization could be grouped.
//路由分组可按 前缀 分组，或者按使用 同一个 中间件（如授权） 进行 分组
// 给当前 路由分组 增加子路由分组
func (group *RouterGroup) Group(component string, handlers ...HandlerFunc) *RouterGroup {
	prefix := path.Join(group.prefix, component)
	return &RouterGroup{
		Handlers: group.combineHandlers(handlers),
		parent:   group,
		prefix:   prefix,
		engine:   group.engine,
	}
}

// Handle registers a new request handle and middlewares with the given path and method.
// The last handler should be the real handler, the other ones should be middlewares that can and should be shared among different routes.
// See the example code in github.
//
// For GET, POST, PUT, PATCH and DELETE requests the respective shortcut
// functions can be used.
//
// This function is intended for bulk loading and to allow the usage of less
// frequently used, non-standardized or custom methods (e.g. for internal
// communication with a proxy).

// 真正处理 业务的 最终 handles
// 将所有  handle 交给 router.handle 去 执行
func (group *RouterGroup) Handle(method, p string, handlers []HandlerFunc) {
	//拼接出 最终的 url
	p = path.Join(group.prefix, p)
	//将路由组的 中间件 与 handles 放到一起
	handlers = group.combineHandlers(handlers)
	group.engine.router.Handle(method, p, func(w http.ResponseWriter, req *http.Request, params httprouter.Params) {
		group.createContext(w, req, params, handlers).Next()
	})
}

// POST is a shortcut for router.Handle("POST", path, handle)
func (group *RouterGroup) POST(path string, handlers ...HandlerFunc) {
	group.Handle("POST", path, handlers)
}

// GET is a shortcut for router.Handle("GET", path, handle)
func (group *RouterGroup) GET(path string, handlers ...HandlerFunc) {
	group.Handle("GET", path, handlers)
}

// DELETE is a shortcut for router.Handle("DELETE", path, handle)
func (group *RouterGroup) DELETE(path string, handlers ...HandlerFunc) {
	group.Handle("DELETE", path, handlers)
}

// PATCH is a shortcut for router.Handle("PATCH", path, handle)
func (group *RouterGroup) PATCH(path string, handlers ...HandlerFunc) {
	group.Handle("PATCH", path, handlers)
}

// PUT is a shortcut for router.Handle("PUT", path, handle)
func (group *RouterGroup) PUT(path string, handlers ...HandlerFunc) {
	group.Handle("PUT", path, handlers)
}

func (group *RouterGroup) combineHandlers(handlers []HandlerFunc) []HandlerFunc {
	s := len(group.Handlers) + len(handlers)
	h := make([]HandlerFunc, 0, s)
	h = append(h, group.Handlers...)
	h = append(h, handlers...)
	return h
}

/************************************/
/****** FLOW AND ERROR MANAGEMENT****/
/************************************/

// Next should be used only in the middlewares.
// It executes the pending handlers in the chain inside the calling handler.
// See example in github.
func (c *Context) Next() {
	c.index++
	s := int8(len(c.handlers))
	for ; c.index < s; c.index++ {
		c.handlers[c.index](c)
	}
}

// Forces the system to do not continue calling the pending handlers.
// For example, the first handler checks if the request is authorized. If it's not, context.Abort(401) should be called.
// The rest of pending handlers would never be called for that request.

//中断后续中间件的执行，如权限校验未通过等情况
func (c *Context) Abort(code int) {
	c.Writer.WriteHeader(code)
	c.index = AbortIndex
}

// Fail is the same than Abort plus an error message.
// Calling `context.Fail(500, err)` is equivalent to:
// ```
// context.Error("Operation aborted", err)
// context.Abort(500)
// ```
func (c *Context) Fail(code int, err error) {
	c.Error(err, "Operation aborted")
	c.Abort(code)
}

// Attachs an error to the current context. The error is pushed to a list of errors.
// It's a gooc idea to call Error for each error ocurred during the resolution of a request.
// A middleware can be used to collect all the errors and push them to a database together, print a log, or append it in the HTTP response.
func (c *Context) Error(err error, meta interface{}) {
	c.Errors = append(c.Errors, ErrorMsg{
		Message: err.Error(),
		Meta:    meta,
	})
}

/************************************/
/******** METADATA MANAGEMENT********/
/************************************/

// Sets a new pair key/value just for the specefied context.
// It also lazy initializes the hashmap
func (c *Context) Set(key string, item interface{}) {
	if c.Keys == nil {
		c.Keys = make(map[string]interface{})
	}
	c.Keys[key] = item
}

// Returns the value for the given key.
// It panics if the value doesn't exist.
func (c *Context) Get(key string) interface{} {
	var ok bool
	var item interface{}
	if c.Keys != nil {
		item, ok = c.Keys[key]
	} else {
		item, ok = nil, false
	}
	if !ok || item == nil {
		log.Panicf("Key %s doesn't exist", key)
	}
	return item
}

/************************************/
/******** ENCOGING MANAGEMENT********/
/************************************/

// Like ParseBody() but this method also writes a 400 error if the json is not valid.
func (c *Context) EnsureBody(item interface{}) bool {
	if err := c.ParseBody(item); err != nil {
		c.Fail(400, err)
		return false
	}
	return true
}

// Parses the body content as a JSON input. It decodes the json payload into the struct specified as a pointer.
func (c *Context) ParseBody(item interface{}) error {
	decoder := json.NewDecoder(c.Req.Body)
	if err := decoder.Decode(&item); err == nil {
		return Validate(c, item)
	} else {
		return err
	}
}

// Serializes the given struct as a JSON into the response body in a fast and efficient way.
// It also sets the Content-Type as "application/json"

// 序列化 json  ,
func (c *Context) JSON(code int, obj interface{}) {
	if code >= 0 {
		c.Writer.WriteHeader(code)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(c.Writer)
	if err := encoder.Encode(obj); err != nil {
		c.Error(err, obj)
		http.Error(c.Writer, err.Error(), 500)
	}
}

// Serializes the given struct as a XML into the response body in a fast and efficient way.
// It also sets the Content-Type as "application/xml"
func (c *Context) XML(code int, obj interface{}) {
	if code >= 0 {
		c.Writer.WriteHeader(code)
	}
	c.Writer.Header().Set("Content-Type", "application/xml")
	encoder := xml.NewEncoder(c.Writer)
	if err := encoder.Encode(obj); err != nil {
		c.Error(err, obj)
		http.Error(c.Writer, err.Error(), 500)
	}
}

// Renders the HTTP template specified by his file name.
// It also update the HTTP code and sets the Content-Type as "text/html".
// See http://golang.org/doc/articles/wiki/
func (c *Context) HTML(code int, name string, data interface{}) {
	if code >= 0 {
		c.Writer.WriteHeader(code)
	}
	c.Writer.Header().Set("Content-Type", "text/html")
	if err := c.engine.HTMLTemplates.ExecuteTemplate(c.Writer, name, data); err != nil {
		c.Error(err, map[string]interface{}{
			"name": name,
			"data": data,
		})
		http.Error(c.Writer, err.Error(), 500)
	}
}

// Writes the given string into the response body and sets the Content-Type to "text/plain"
func (c *Context) String(code int, msg string) {
	c.Writer.Header().Set("Content-Type", "text/plain")
	c.Writer.WriteHeader(code)
	c.Writer.Write([]byte(msg))
}

// Writes some data into the body stream and updates the HTTP code
func (c *Context) Data(code int, data []byte) {
	c.Writer.WriteHeader(code)
	c.Writer.Write(data)
}
