package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/imyousuf/appcommons/config"
	"github.com/rs/xid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"github.com/rs/zerolog/log"

	"github.com/julienschmidt/httprouter"
)

const (
	PreviousPaginationQueryParamKey = "previous"
	NextPaginationQueryParamKey     = "next"
	FormDataContentTypeHeaderValue  = "application/x-www-form-urlencoded"
	JSONContentTypeHeaderValue      = "application/json"
	HeaderContentType               = "Content-Type"
	HeaderUnmodifiedSince           = "If-Unmodified-Since"
	HeaderLastModified              = "Last-Modified"
	HeaderRequestID                 = "X-Request-ID"
	requestIDLogFieldKey            = "requestId"
)

var (
	listener ServerLifecycleListener
	server   *http.Server
	// ErrUnsupportedMediaType is returned when client does not provide appropriate `Content-Type` header
	ErrUnsupportedMediaType = errors.New("media type not supported")
	// ErrConditionalFailed is returned when update is missing `If-Unmodified-Since` header
	ErrConditionalFailed = errors.New("update failed due to mismatch of `If-Unmodified-Since` header value")
	// ErrNotFound is returned when resource is not found
	ErrNotFound = errors.New("request resource not found")
	// ErrBadRequest is returned when protocol for a PUT/POST/DELETE request is not met
	ErrBadRequest = errors.New("bad request: Update is missing `If-Unmodified-Since` header ")
	// ErrBadRequestForRequeue is returned when requeue form param does not match consumer token
	ErrBadRequestForRequeue = errors.New("`requeue` form param must match consumer token")
	getJSON                 = func(buf *bytes.Buffer, data interface{}) error {
		return json.NewEncoder(buf).Encode(data)
	}
)

type (
	// ServerLifecycleListener listens to key server lifecycle error
	ServerLifecycleListener interface {
		StartingServer()
		ServerStartFailed(err error)
		ServerShutdownCompleted()
	}

	// EndpointController represents very basic functionality of an endpoint
	EndpointController interface {
		GetPath() string
		FormatAsRelativeLink(params ...httprouter.Param) string
	}

	// Get represents GET Method Call to a resource
	Get interface {
		Get(w http.ResponseWriter, r *http.Request, ps httprouter.Params)
	}

	// Put represents PUT Method Call to a resource
	Put interface {
		Put(w http.ResponseWriter, r *http.Request, ps httprouter.Params)
	}

	// Post represents POST Method Call to a resource
	Post interface {
		Post(w http.ResponseWriter, r *http.Request, ps httprouter.Params)
	}

	// Delete represents DELETE Method Call to a resource
	Delete interface {
		Delete(w http.ResponseWriter, r *http.Request, ps httprouter.Params)
	}

	idKey struct{}
)

// NotifyOnInterrupt registers channel to get notified when interrupt is captured
var NotifyOnInterrupt = func(stop *chan os.Signal) {
	signal.Notify(*stop, os.Interrupt, os.Kill, syscall.SIGTERM)
}

func getRequestID(r *http.Request) (requestID string, request *http.Request) {
	ctx := r.Context()
	requestID, ok := ctx.Value(idKey{}).(string)
	request = r
	if !ok {
		requestID = r.Header.Get(HeaderRequestID)
		if len(requestID) < 1 {
			requestID = xid.New().String()
		}
		ctx = context.WithValue(ctx, idKey{}, requestID)
		request = r.WithContext(ctx)
	}
	return requestID, request
}

// getRequestIDHandler is similar to hlog.RequestIDHandler just the twist is it expects string as request id and not xid.ID
func getRequestIDHandler(fieldKey, headerName string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID, request := getRequestID(r)
			ctx := request.Context()
			log := zerolog.Ctx(ctx)
			if len(fieldKey) > 0 {
				log.UpdateContext(func(c zerolog.Context) zerolog.Context {
					return c.Str(fieldKey, requestID)
				})
			}
			if len(headerName) > 0 {
				w.Header().Set(headerName, requestID)
			}
			next.ServeHTTP(w, request)
		})
	}
}

func logAccess(r *http.Request, status, size int, duration time.Duration) {
	hlog.FromRequest(r).Info().
		Str("method", r.Method).
		Str("url", r.URL.String()).
		Int("status", status).
		Int("size", size).
		Dur("duration", duration).
		Msg("")
}

func getHandler(apiRouter *httprouter.Router) http.Handler {
	// Chain handlers - new handler to attach logger to request context, request id handler and lastly access log handler all ending with the our routes
	return hlog.NewHandler(log.Logger)(getRequestIDHandler(requestIDLogFieldKey, HeaderRequestID)(hlog.AccessHandler(logAccess)(apiRouter)))
}

// ConfigureAPI configures API Server with interrupt handling
func ConfigureAPI(httpConfig config.HTTPConfig, iListener ServerLifecycleListener, apiRouter *httprouter.Router) *http.Server {
	listener = iListener
	handler := getHandler(apiRouter)
	server = &http.Server{
		Handler:      handler,
		Addr:         httpConfig.GetHTTPListeningAddr(),
		ReadTimeout:  httpConfig.GetHTTPReadTimeout(),
		WriteTimeout: httpConfig.GetHTTPWriteTimeout(),
	}
	go func() {
		log.Print("Listening to http at -", httpConfig.GetHTTPListeningAddr())
		iListener.StartingServer()
		if serverListenErr := server.ListenAndServe(); serverListenErr != nil {
			iListener.ServerStartFailed(serverListenErr)
			log.Print(serverListenErr)
		}
	}()
	stop := make(chan os.Signal, 1)
	NotifyOnInterrupt(&stop)
	go func() {
		<-stop
		handleExit()
	}()
	return server
}

func handleExit() {
	log.Print("Shutting down the server...")
	serverShutdownContext, shutdownTimeoutCancelFunc := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownTimeoutCancelFunc()
	server.Shutdown(serverShutdownContext)
	log.Print("Server gracefully stopped!")
	listener.ServerShutdownCompleted()
}

func SetupAPIRoutes(apiRouter *httprouter.Router, endpoints ...EndpointController) {
	for _, endpoint := range endpoints {
		getEndpoint, ok := endpoint.(Get)
		if ok {
			apiRouter.GET(endpoint.GetPath(), getEndpoint.Get)
		}
		putEndpoint, ok := endpoint.(Put)
		if ok {
			apiRouter.PUT(endpoint.GetPath(), putEndpoint.Put)
		}
		postEndpoint, ok := endpoint.(Post)
		if ok {
			apiRouter.POST(endpoint.GetPath(), postEndpoint.Post)
		}
		deleteEndpoint, ok := endpoint.(Delete)
		if ok {
			apiRouter.DELETE(endpoint.GetPath(), deleteEndpoint.Delete)
		}
	}
}

func WriteErr(w http.ResponseWriter, err error) {
	WriteStatus(w, http.StatusInternalServerError, err)
}

func WriteNotFound(w http.ResponseWriter) {
	WriteStatus(w, http.StatusNotFound, ErrNotFound)
}

func WriteBadRequest(w http.ResponseWriter) {
	WriteStatus(w, http.StatusBadRequest, ErrBadRequest)
}

func WriteUnsupportedMediaType(w http.ResponseWriter) {
	WriteStatus(w, http.StatusUnsupportedMediaType, ErrUnsupportedMediaType)
}

func WritePreconditionFailed(w http.ResponseWriter) {
	WriteStatus(w, http.StatusPreconditionFailed, ErrUnsupportedMediaType)
}

func WriteStatus(w http.ResponseWriter, code int, err error) {
	w.WriteHeader(code)
	if err != nil {
		w.Write([]byte(err.Error()))
	}
}

func WriteJSON(w http.ResponseWriter, data interface{}) {
	// Write JSON
	var buf bytes.Buffer
	err := getJSON(&buf, data)
	if err != nil {
		// return error
		WriteErr(w, err)
		return
	}
	w.WriteHeader(200)
	w.Header().Add(HeaderContentType, JSONContentTypeHeaderValue)
	w.Write(buf.Bytes())
}

func FormatURL(params []httprouter.Param, urlTemplate string, urlParamNames ...string) (result string) {
	paramValues := make(map[string]string)
	for _, paramName := range urlParamNames {
		if val := findParam(params, paramName); len(val) > 0 {
			paramValues[paramName] = val
		}
	}
	result = urlTemplate
	for key, value := range paramValues {
		result = strings.ReplaceAll(result, ":"+key, value)
	}
	return result
}

func findParam(params httprouter.Params, name string) string {
	return params.ByName(name)
}
