package gin

import (
	"fmt"
	"log"
	"time"
)

//错误处理中间件 ，如果其内部的调用有错误被抛出，则
//日志中间件，打印请求用时及错误信息

func ErrorLogger() HandlerFunc {
	return func(c *Context) {
		c.Next()

		if len(c.Errors) > 0 {
			// -1 status code = do not change current one
			c.JSON(-1, c.Errors)
		}
	}
}

func Logger() HandlerFunc {
	return func(c *Context) {

		// Start timer
		t := time.Now()

		// Process request
		c.Next()

		// Calculate resolution time
		log.Printf("%s in %v", c.Req.RequestURI, time.Since(t))
		if len(c.Errors) > 0 {
			fmt.Println(c.Errors)
		}
	}
}
