package di

import (
	"net/http"
	"time"
)

func (c *Container) GetHTTPServer() *http.Server {
	if c.httpServer == nil {
		router := http.NewServeMux()

		router.Handle("/write", c.getWriteHTTPHandler())
		router.Handle("/health", c.getHealthHTTPHandler())
		router.Handle("/read", c.getReadHTTPHandler())

		c.httpServer = &http.Server{
			Addr:              c.cfg.HTTP.Addr,
			Handler:           router,
			ReadHeaderTimeout: time.Second,
		}
	}

	return c.httpServer
}
