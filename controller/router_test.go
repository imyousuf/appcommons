package controller

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/imyousuf/appcommons/config"
	"github.com/julienschmidt/httprouter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type ServerLifecycleListenerMockImpl struct {
	mock.Mock
	serverListener chan bool
}

func (m *ServerLifecycleListenerMockImpl) StartingServer()             { m.Called() }
func (m *ServerLifecycleListenerMockImpl) ServerStartFailed(err error) { m.Called(err) }
func (m *ServerLifecycleListenerMockImpl) ServerShutdownCompleted() {
	m.Called()
	m.serverListener <- true
}

var forceServerExiter = func(stop *chan os.Signal) {
	go func() {
		var client = &http.Client{Timeout: time.Second * 10}
		defer func() {
			client.CloseIdleConnections()
		}()
		for {
			response, err := client.Get("http://localhost:17654/_status")
			if err == nil {
				if response.StatusCode == 200 {
					break
				}
			}
		}
		*stop <- os.Interrupt
	}()
}

func TestConfigureAPI(t *testing.T) {
	mListener := &ServerLifecycleListenerMockImpl{serverListener: make(chan bool)}
	oldNotify := NotifyOnInterrupt
	NotifyOnInterrupt = forceServerExiter
	configuration, _, _ := config.GetAutoConfiguration()
	mListener.On("StartingServer").Return()
	mListener.On("ServerStartFailed", mock.Anything).Return()
	mListener.On("ServerShutdownCompleted").Return()
	apiRouter := httprouter.New()
	apiRouter.GET("/_status", func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		jsonData := make(map[string]string)
		jsonData["TestProp"] = "Sample Value"
		WriteJSON(rw, jsonData)
	})
	server := ConfigureAPI(configuration, mListener, apiRouter)
	<-mListener.serverListener
	assert.NotNil(t, server)
	mListener.AssertExpectations(t)
	defer func() { NotifyOnInterrupt = oldNotify }()
}

func waitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return false // completed normally
	case <-time.After(timeout):
		return true // timed out
	}
}

func TestNotifyOnInterrupt(t *testing.T) {
	stop := make(chan os.Signal, 1)
	defer close(stop)
	NotifyOnInterrupt(&stop)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		<-stop
		wg.Done()
	}()
	syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	if waitTimeout(&wg, 100*time.Millisecond) {
		t.Fail()
	}
}

type ClientErrorController struct {
}

func (controller *ClientErrorController) Get(w http.ResponseWriter, r *http.Request, param httprouter.Params) {
	WriteBadRequest(w)
}

func (controller *ClientErrorController) Put(w http.ResponseWriter, r *http.Request, param httprouter.Params) {
	WriteNotFound(w)
}

func (controller *ClientErrorController) Post(w http.ResponseWriter, r *http.Request, param httprouter.Params) {
	WritePreconditionFailed(w)
}

func (controller *ClientErrorController) Delete(w http.ResponseWriter, r *http.Request, param httprouter.Params) {
	WriteUnsupportedMediaType(w)
}

// GetPath returns the endpoint's path
func (controller *ClientErrorController) GetPath() string {
	return "/client"
}

// FormatAsRelativeLink Format as relative URL of this resource based on the params
func (controller *ClientErrorController) FormatAsRelativeLink(params ...httprouter.Param) string {
	return controller.GetPath()
}

type ServerErrorController struct {
}

func (controller *ServerErrorController) Get(w http.ResponseWriter, r *http.Request, param httprouter.Params) {
	WriteErr(w, errors.New("server side error"))
}

// GetPath returns the endpoint's path
func (controller *ServerErrorController) GetPath() string {
	return "/server"
}

// FormatAsRelativeLink Format as relative URL of this resource based on the params
func (controller *ServerErrorController) FormatAsRelativeLink(params ...httprouter.Param) string {
	return controller.GetPath()
}

func TestWriteHelpers(t *testing.T) {
	apiRouter := httprouter.New()
	SetupAPIRoutes(apiRouter, &ClientErrorController{}, &ServerErrorController{})
	testRouter := getHandler(apiRouter)
	getReq, _ := http.NewRequest("GET", "/client", nil)
	getResp := httptest.NewRecorder()
	testRouter.ServeHTTP(getResp, getReq)
	assert.Equal(t, 400, getResp.Code)
	putReq, _ := http.NewRequest("PUT", "/client", nil)
	putResp := httptest.NewRecorder()
	testRouter.ServeHTTP(putResp, putReq)
	assert.Equal(t, 404, putResp.Code)
	postReq, _ := http.NewRequest("POST", "/client", nil)
	postResp := httptest.NewRecorder()
	testRouter.ServeHTTP(postResp, postReq)
	assert.Equal(t, 412, postResp.Code)
	delReq, _ := http.NewRequest("DELETE", "/client", nil)
	delResp := httptest.NewRecorder()
	testRouter.ServeHTTP(delResp, delReq)
	assert.Equal(t, 415, delResp.Code)
	getServReq, _ := http.NewRequest("GET", "/server", nil)
	getServResp := httptest.NewRecorder()
	testRouter.ServeHTTP(getServResp, getServReq)
	assert.Equal(t, 500, getServResp.Code)
}

func TestGetJSONErr(t *testing.T) {
	apiRouter := httprouter.New()
	apiRouter.GET("/_status", func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		jsonData := make(map[string]string)
		jsonData["TestProp"] = "Sample Value"
		oldGetJSON := getJSON
		getJSON = func(buf *bytes.Buffer, data interface{}) error {
			return errors.New("json error")
		}
		WriteJSON(rw, jsonData)
		getJSON = oldGetJSON
	})
	SetupAPIRoutes(apiRouter, &ClientErrorController{}, &ServerErrorController{})
	testRouter := getHandler(apiRouter)
	getReq, _ := http.NewRequest("GET", "/_status", nil)
	getResp := httptest.NewRecorder()
	testRouter.ServeHTTP(getResp, getReq)
	assert.Equal(t, 500, getResp.Code)
}

func TestFormatURL(t *testing.T) {
	params := make([]httprouter.Param, 3)
	paramKeys := []string{"test1", "test2", "test3"}
	params[0] = httprouter.Param{Key: paramKeys[0], Value: "one"}
	params[1] = httprouter.Param{Key: paramKeys[1], Value: "two"}
	params[2] = httprouter.Param{Key: paramKeys[2], Value: "three"}
	resultURL := FormatURL(params, "/s/:test1/:test2/:test3", paramKeys...)
	assert.Equal(t, "/s/one/two/three", resultURL)
}
