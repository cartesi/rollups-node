// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type HttpServiceSuite struct {
	suite.Suite
	ServicePort int
	ServiceAddr string
}

func TestHttpService(t *testing.T) {
	suite.Run(t, new(HttpServiceSuite))
}

func (s *HttpServiceSuite) SetupSuite() {
	s.ServicePort = 5555
}

func (s *HttpServiceSuite) SetupTest() {
	s.ServicePort++
	s.ServiceAddr = fmt.Sprintf("127.0.0.1:%v", s.ServicePort)
}

func (s *HttpServiceSuite) TestItStopsWhenContextIsClosed() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	service := HttpService{Name: "http", Address: s.ServiceAddr, Handler: http.NewServeMux()}

	result := make(chan error, 1)
	ready := make(chan struct{}, 1)
	go func() {
		result <- service.Start(ctx, ready)
	}()

	select {
	case <-ready:
		cancel()
	case <-time.After(DefaultServiceTimeout):
		s.FailNow("timed out waiting for HttpService to be ready")
	}

	select {
	case err := <-result:
		s.Nil(err)
	case <-time.After(DefaultServiceTimeout):
		s.FailNow("timed out waiting for HttpService to stop")
	}
}

func (s *HttpServiceSuite) TestItRespondsToRequests() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	router := http.NewServeMux()
	router.HandleFunc("/test", defaultHandler)
	service := HttpService{Name: "http", Address: s.ServiceAddr, Handler: router}

	result := make(chan error, 1)
	ready := make(chan struct{}, 1)
	go func() {
		result <- service.Start(ctx, ready)
	}()

	select {
	case <-ready:
	case <-time.After(DefaultServiceTimeout):
		s.FailNow("timed out waiting for HttpService to be ready")
	}

	resp, err := http.Get(fmt.Sprintf("http://%v/test", s.ServiceAddr))
	if err != nil {
		s.FailNow(err.Error())
	}
	s.assertResponse(resp)
}

func (s *HttpServiceSuite) TestItRespondsOngoingRequestsAfterContextIsClosed() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	router := http.NewServeMux()
	router.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// simulate a long-running request
		<-time.After(100 * time.Millisecond)
		fmt.Fprintf(w, "test")
	})
	service := HttpService{Name: "http", Address: s.ServiceAddr, Handler: router}

	result := make(chan error, 1)
	ready := make(chan struct{}, 1)
	go func() {
		result <- service.Start(ctx, ready)
	}()

	select {
	case <-ready:
	case <-time.After(DefaultServiceTimeout):
		s.FailNow("timed out wating for HttpService to be ready")
	}

	clientResult := make(chan ClientResult, 1)
	go func() {
		resp, err := http.Get(fmt.Sprintf("http://%v/test", s.ServiceAddr))
		clientResult <- ClientResult{Response: resp, Error: err}
	}()

	// wait a bit so server has enough time to start responding the request
	<-time.After(200 * time.Millisecond)
	cancel()

	select {
	case res := <-clientResult:
		s.Nil(res.Error)
		s.assertResponse(res.Response)
		err := <-result
		s.Nil(err)
	case <-result:
		s.FailNow("HttpService closed before responding")
	}
}

type ClientResult struct {
	Response *http.Response
	Error    error
}

func defaultHandler(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "test")
}

func (s *HttpServiceSuite) assertResponse(resp *http.Response) {
	s.Equal(http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		s.FailNow("failed to read response body. ", err)
	}
	s.Equal([]byte("test"), bytes)
}
