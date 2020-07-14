package distributed_locker

import (
	"fmt"
	"net/http"

	"github.com/werf/lockgate"

	"github.com/werf/lockgate/pkg/util"
)

type HttpBackend struct {
	URLEndpoint string
	HttpClient  *http.Client
}

func NewHttpBackend(urlEndpoint string) *HttpBackend {
	return &HttpBackend{
		URLEndpoint: urlEndpoint,
		HttpClient:  &http.Client{},
	}
}

func (backend *HttpBackend) Acquire(lockName string, opts AcquireOptions) (lockgate.LockHandle, error) {
	var request = AcquireRequest{
		LockName: lockName,
		Opts:     opts,
	}
	var response AcquireResponse

	if err := util.PerformHttpPost(backend.HttpClient, fmt.Sprintf("%s/%s", backend.URLEndpoint, "acquire"), request, &response); err != nil {
		return lockgate.LockHandle{}, err
	}
	return response.LockHandle, response.Err.Error
}

func (backend *HttpBackend) RenewLease(handle lockgate.LockHandle) error {
	var request = RenewLeaseRequest{LockHandle: handle}
	var response RenewLeaseResponse

	if err := util.PerformHttpPost(backend.HttpClient, fmt.Sprintf("%s/%s", backend.URLEndpoint, "renew-lease"), request, &response); err != nil {
		return err
	}
	return response.Err.Error
}

func (backend *HttpBackend) Release(handle lockgate.LockHandle) error {
	var request = ReleaseRequest{LockHandle: handle}
	var response ReleaseResponse

	if err := util.PerformHttpPost(backend.HttpClient, fmt.Sprintf("%s/%s", backend.URLEndpoint, "release"), request, &response); err != nil {
		return err
	}
	return response.Err.Error
}
