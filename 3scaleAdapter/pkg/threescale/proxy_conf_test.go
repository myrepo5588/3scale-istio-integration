package threescale

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/3scale/3scale-go-client/fake"
	"github.com/3scale/3scale-porta-go-client/client"
	sysFake "github.com/3scale/3scale-porta-go-client/fake"
	pb "github.com/3scale/istio-integration/3scaleAdapter/config"
	"github.com/gogo/protobuf/types"

	"istio.io/api/mixer/adapter/model/v1beta1"
	"istio.io/istio/mixer/template/authorization"
)

func TestProxyConfigCacheFlushing(t *testing.T) {
	const ttl = time.Duration(time.Millisecond * 100)
	type (
		testInput struct {
			name     string
			params   pb.Params
			template authorization.InstanceMsg
		}

		testResult struct {
			result *v1beta1.CheckResult
			err    error
		}
	)

	var (
		proxyConf         client.ProxyConfigElement
		fetchedFromRemote int
	)

	ctx := context.TODO()
	httpClient := NewTestClient(func(req *http.Request) *http.Response {
		if req.URL.Host == "www.fake-system.3scale.net:443" {
			fetchedFromRemote++
			return sysFake.GetProxyConfigLatestSuccess()
		} else {

			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(bytes.NewBufferString(fake.GetAuthSuccess())),
				Header:     make(http.Header),
			}
		}
	})

	// Create cache manager and populate
	pc := NewProxyConfigCache(time.Duration(ttl), time.Duration(time.Second*1), 3)
	proxyConf = unmarshalConfig(t)

	cfg := &pb.Params{ServiceId: "123", SystemUrl: "https://www.fake-system.3scale.net"}
	cacheKey := pc.getCacheKeyFromCfg(cfg)
	pc.set(cacheKey, proxyConf, cacheRefreshStore{})
	conf := &AdapterConfig{systemCache: pc}
	c := &Threescale{client: httpClient, conf: conf}

	inputs := []testInput{
		{
			name: "One",
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "happy-path",
			},
			template: authorization.InstanceMsg{
				Name: "",
				Subject: &authorization.SubjectMsg{
					User: "secret",
				},
				Action: &authorization.ActionMsg{
					Path:   "/?user_key=secret",
					Method: "get",
				},
			},
		},
		{
			name: "Two",
			params: pb.Params{
				ServiceId:   "321",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "happy-path",
			},
			template: authorization.InstanceMsg{
				Name: "",
				Subject: &authorization.SubjectMsg{
					User: "secret",
				},
				Action: &authorization.ActionMsg{
					Path:   "/?user_key=secret",
					Method: "get",
				},
			},
		},
	}

	resultOne := make(chan testResult)
	resultTwo := make(chan testResult)
	results := []chan testResult{resultOne, resultTwo}

	for i, input := range inputs {
		copy := testInput{input.name, input.params, input.template}
		index := i
		go func(input testInput, index int) {
			r := &authorization.HandleAuthorizationRequest{
				Instance: &authorization.InstanceMsg{
					Subject: &authorization.SubjectMsg{},
				},
				AdapterConfig: &types.Any{},
				DedupId:       "",
			}

			b, _ := input.params.Marshal()
			r.AdapterConfig.Value = b
			r.Instance = &input.template

			result, err := c.HandleAuthorization(ctx, r)
			results[index] <- testResult{result, err}
		}(copy, index)
	}

	assert := func(msg testResult) {
		if msg.result.Status.Code != 0 {
			t.Fatalf("expected all results to succeed")
		}
	}

	for i := 0; i < len(inputs); i++ {
		select {
		case message := <-resultOne:
			assert(message)
		case message := <-resultTwo:
			assert(message)
		}
	}
	if fetchedFromRemote != 1 {
		t.Fatalf("expected only one result not fetched from cache")
	}

	testStopNotStartedErr := c.conf.systemCache.StopFlushWorker()
	if testStopNotStartedErr == nil {
		t.Fatalf("expected to get error when stopping unstarted worker")
	}

	c.conf.systemCache.StartFlushWorker()

	testStartErr := c.conf.systemCache.StartFlushWorker()
	if testStartErr == nil {
		t.Fatalf("expected only one worker to start")
	}

	<-time.After(time.Second)
	if len(c.conf.systemCache.cache) > 0 {
		t.Fatalf("expected cache to be empty")
	}
	c.conf.systemCache.StopFlushWorker()

	testStartErr = c.conf.systemCache.StartFlushWorker()
	if testStartErr != nil {
		t.Fatalf("expected to be able to restart worker")
	}

}

func TestProxyConfigCacheRefreshing(t *testing.T) {
	const ttl = time.Duration(time.Second * 10)
	type (
		testInput struct {
			name     string
			params   pb.Params
			template authorization.InstanceMsg
		}

		testResult struct {
			result *v1beta1.CheckResult
			err    error
		}
	)

	var (
		proxyConf         client.ProxyConfigElement
		fetchedFromRemote int
	)

	ctx := context.TODO()
	httpClient := NewTestClient(func(req *http.Request) *http.Response {
		if req.URL.Host == "www.fake-system.3scale.net:443" {
			fetchedFromRemote++
			return sysFake.GetProxyConfigLatestSuccess()
		} else {

			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(bytes.NewBufferString(fake.GetAuthSuccess())),
				Header:     make(http.Header),
			}
		}
	})

	// Create cache manager and populate
	pc := NewProxyConfigCache(time.Duration(ttl), time.Duration(ttl), 3)
	proxyConf = unmarshalConfig(t)
	conf := &AdapterConfig{systemCache: pc}
	c := &Threescale{client: httpClient, conf: conf}
	sysClient, err := c.systemClientBuilder("https://www.fake-system.3scale.net")
	if err != nil {
		t.Fatalf("unexpected error builoding system client")
	}

	cfg := &pb.Params{ServiceId: "123", SystemUrl: "https://www.fake-system.3scale.net"}
	cacheKey := pc.getCacheKeyFromCfg(cfg)
	pc.set(cacheKey, proxyConf, cacheRefreshStore{
		cfg:    cfg,
		client: sysClient,
	})

	inputs := []testInput{
		{
			name: "One",
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "happy-path",
			},
			template: authorization.InstanceMsg{
				Name: "",
				Subject: &authorization.SubjectMsg{
					User: "secret",
				},
				Action: &authorization.ActionMsg{
					Path:   "/?user_key=secret",
					Method: "get",
				},
			},
		},
		{
			name: "Two",
			params: pb.Params{
				ServiceId:   "321",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "happy-path",
			},
			template: authorization.InstanceMsg{
				Name: "",
				Subject: &authorization.SubjectMsg{
					User: "secret",
				},
				Action: &authorization.ActionMsg{
					Path:   "/?user_key=secret",
					Method: "get",
				},
			},
		},
	}

	resultOne := make(chan testResult)
	resultTwo := make(chan testResult)
	results := []chan testResult{resultOne, resultTwo}

	for i, input := range inputs {
		copy := testInput{input.name, input.params, input.template}
		index := i
		go func(input testInput, index int) {
			r := &authorization.HandleAuthorizationRequest{
				Instance: &authorization.InstanceMsg{
					Subject: &authorization.SubjectMsg{},
				},
				AdapterConfig: &types.Any{},
				DedupId:       "",
			}

			b, _ := input.params.Marshal()
			r.AdapterConfig.Value = b
			r.Instance = &input.template

			result, err := c.HandleAuthorization(ctx, r)
			results[index] <- testResult{result, err}
		}(copy, index)
	}

	assert := func(msg testResult) {
		if msg.result.Status.Code != 0 {
			t.Fatalf("expected all results to succeed")
		}
	}

	for i := 0; i < len(inputs); i++ {
		select {
		case message := <-resultOne:
			assert(message)
		case message := <-resultTwo:
			assert(message)
		}
	}
	if fetchedFromRemote != 1 {
		t.Fatalf("expected only one result not fetched from cache")
	}

	err = c.conf.systemCache.StartRefreshWorker()
	if err != nil {
		t.Fatalf("expected to be able to start the refresh worker")
	}

	err = c.conf.systemCache.StartRefreshWorker()
	if err == nil {
		t.Fatalf("expected error when calling to start the refresh worker a second time")
	}

	<-time.After(time.Second)
	if fetchedFromRemote < 3 {
		t.Fatalf("expected cache to have been refreshed")
	}

	err = c.conf.systemCache.StopRefreshWorker()
	if err != nil {
		t.Fatalf("unexpected error when stopping refresh worker")
	}

	err = c.conf.systemCache.StopRefreshWorker()
	if err == nil {
		t.Fatalf("unexpected error when stopping refresh worker again")
	}
}

func unmarshalConfig(t *testing.T) client.ProxyConfigElement {
	t.Helper()
	var proxyConf client.ProxyConfigElement
	if err := json.Unmarshal([]byte(sysFake.GetProxyConfigLatestJson()), &proxyConf); err != nil {
		t.Fatalf("failed to unmarsahl proxy conf")
	}
	return proxyConf
}
