// Copyright 2019 The OpenPitrix Authors. All rights reserved.
// Use of this source code is governed by a Apache license
// that can be found in the LICENSE file.
package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"golang.org/x/tools/godoc/vfs"
	"golang.org/x/tools/godoc/vfs/httpfs"
	"golang.org/x/tools/godoc/vfs/mapfs"
	"google.golang.org/grpc"
	"openpitrix.io/logger"

	staticSpec "openpitrix.io/notification/pkg/apigateway/spec"
	staticSwaggerUI "openpitrix.io/notification/pkg/apigateway/swagger-ui"
	"openpitrix.io/notification/pkg/config"
	"openpitrix.io/notification/pkg/pb"
)

type register struct {
	f        func(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) (err error)
	endpoint string
}

func ServeApiGateway() {
	//version.PrintVersionInfo(func(s string, i ...interface{}) {
	//	logger.Infof(nil, s, i...)
	//})

	cfg := config.GetInstance()
	logger.Infof(nil, "Notification service http://%s:%d", cfg.App.Host, cfg.App.Port)
	logger.Infof(nil, "Api service start http://%s:%d/swagger-ui/", cfg.App.ApiHost, cfg.App.ApiPort)

	s := Server{}

	if err := s.run(); err != nil {
		logger.Criticalf(nil, "Api gateway run failed: %+v", err)
		panic(err)
	}
}

//const (
//	Authorization = "Authorization"
//	RequestIdKey  = "X-Request-Id"
//)

func log() gin.HandlerFunc {
	l := logger.New()
	l.HideCallstack()
	return func(c *gin.Context) {
		requestID := uuid.New()
		//c.Request.Header.Set(RequestIdKey, requestID)
		//c.Writer.Header().Set(RequestIdKey, requestID)

		t := time.Now()

		// process request
		c.Next()

		latency := time.Since(t)
		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()
		path := c.Request.URL.Path

		logStr := fmt.Sprintf("%s | %3d | %v | %s | %s %s %s",
			requestID,
			statusCode,
			latency,
			clientIP, method,
			path,
			c.Errors.String(),
		)

		switch {
		case statusCode >= 400 && statusCode <= 499:
			l.Warnf(nil, logStr)
		case statusCode >= 500:
			l.Errorf(nil, logStr)
		default:
			l.Infof(nil, logStr)
		}
	}
}

func recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				httprequest, _ := httputil.DumpRequest(c.Request, false)
				logger.Criticalf(nil, "Panic recovered: %+v\n%s", err, string(httprequest))
				c.JSON(500, gin.H{
					"title": "Error",
					"err":   err,
				})
			}
		}()
		c.Next() // execute all the handlers
	}
}

func handleSwagger() http.Handler {
	ns := vfs.NameSpace{}
	ns.Bind("/", mapfs.New(staticSwaggerUI.Files), "/", vfs.BindReplace)
	ns.Bind("/", mapfs.New(staticSpec.Files), "/", vfs.BindBefore)
	return http.StripPrefix("/swagger-ui", http.FileServer(httpfs.New(ns)))
}

func (s *Server) run() error {
	gin.SetMode(gin.ReleaseMode)
	mainHandler := gin.WrapH(s.mainHandler())

	r := gin.New()
	r.Use(log())
	r.Use(recovery())
	r.Any("/swagger-ui/*filepath", gin.WrapH(handleSwagger()))
	r.Any("/v1/*filepath", mainHandler)
	r.Any("/v2/*filepath", mainHandler)
	r.Any("/api/*filepath", mainHandler)

	cfg := config.GetInstance()
	return r.Run(fmt.Sprintf(":%d", cfg.App.ApiPort))
}

func (s *Server) mainHandler() http.Handler {
	var gwmux = runtime.NewServeMux()
	var opts = []grpc.DialOption{grpc.WithInsecure()}
	var err error

	cfg := config.GetInstance()
	for _, r := range []register{{
		pb.RegisterNotificationHandlerFromEndpoint,
		fmt.Sprintf("localhost:%d", cfg.App.Port),
	}} {
		err = r.f(context.Background(), gwmux, r.endpoint, opts)
		if err != nil {
			err = errors.WithStack(err)
			logger.Errorf(nil, "Dial [%s] failed: %+v", r.endpoint, err)
		}
	}

	mux := http.NewServeMux()
	mux.Handle("/", gwmux)

	return formWrapper(mux)
}

// Ref: https://github.com/grpc-ecosystem/grpc-gateway/issues/7#issuecomment-358569373
func formWrapper(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			jsonMap := make(map[string]interface{}, len(r.Form))
			for k, v := range r.Form {
				if len(v) > 0 {
					jsonMap[k] = v[0]
				}
			}
			jsonBody, err := json.Marshal(jsonMap)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
			}

			r.Body = ioutil.NopCloser(bytes.NewReader(jsonBody))
			r.ContentLength = int64(len(jsonBody))
			r.Header.Set("Content-Type", "application/json")
		}

		h.ServeHTTP(w, r)
	})
}
