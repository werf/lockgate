package distributed_locker

import (
	"fmt"
	"net/http"

	"github.com/werf/lockgate"

	"github.com/werf/lockgate/pkg/util"
)

func RunHttpBackendServer(ip, port string, backend DistributedLockerBackend) error {
	handler := NewHttpBackendHandler(backend)
	return http.ListenAndServe(fmt.Sprintf("%s:%s", ip, port), handler)
}

type HttpBackendHandler struct {
	*http.ServeMux
	Backend DistributedLockerBackend
}

func NewHttpBackendHandler(backend DistributedLockerBackend) *HttpBackendHandler {
	handler := &HttpBackendHandler{
		Backend:  backend,
		ServeMux: http.NewServeMux(),
	}

	handler.HandleFunc("/acquire", handler.handleAcquire)
	handler.HandleFunc("/renew-lease", handler.handleRenewLease)
	handler.HandleFunc("/release", handler.handleRelease)

	return handler
}

func (handler *HttpBackendHandler) handleAcquire(w http.ResponseWriter, r *http.Request) {
	var request AcquireRequest
	var response AcquireResponse
	util.HandleHttpRequest(w, r, &request, &response, func() {
		debug("HttpBackendHandler.Acquire -- request %#v", request)
		response.LockHandle, response.Err.Error = handler.Backend.Acquire(request.LockName, request.Opts)
		debug("HttpBackendHandler.Acquire -- response %#v, err %q", response, response.Err)
	})
}

func (handler *HttpBackendHandler) handleRenewLease(w http.ResponseWriter, r *http.Request) {
	var request RenewLeaseRequest
	var response RenewLeaseResponse
	util.HandleHttpRequest(w, r, &request, &response, func() {
		debug("HttpBackendHandler.RenewLease -- request %#v", request)
		response.Err.Error = handler.Backend.RenewLease(request.LockHandle)
		debug("HttpBackendHandler.RenewLease -- response %#v, err %q", response, response.Err)
	})
}

func (handler *HttpBackendHandler) handleRelease(w http.ResponseWriter, r *http.Request) {
	var request ReleaseRequest
	var response ReleaseResponse
	util.HandleHttpRequest(w, r, &request, &response, func() {
		debug("HttpBackendHandler.Release -- request %#v", request)
		response.Err.Error = handler.Backend.Release(request.LockHandle)
		debug("HttpBackendHandler.Release -- response %#v err=%q", response, response.Err)
	})
}

type AcquireRequest struct {
	LockName string         `json:"lockName"`
	Opts     AcquireOptions `json:"opts"`
}

type AcquireResponse struct {
	LockHandle lockgate.LockHandle    `json:"lockHandle"`
	Err        util.SerializableError `json:"err"`
}

type RenewLeaseRequest struct {
	LockHandle lockgate.LockHandle `json:"lockHandle"`
}

type RenewLeaseResponse struct {
	Err util.SerializableError `json:"err"`
}

type ReleaseRequest struct {
	LockHandle lockgate.LockHandle `json:"lockHandle"`
}

type ReleaseResponse struct {
	Err util.SerializableError `json:"err"`
}
