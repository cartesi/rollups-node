// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package node

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type blockingChildImpl struct {
	service.ServiceTemplate
	started chan struct{}
	done    chan struct{}
	once    sync.Once
}

func (c *blockingChildImpl) OnServe(ctx context.Context) error {
	close(c.started)
	<-ctx.Done()
	c.once.Do(func() { close(c.done) })
	return nil
}

func createBlockingChild(t *testing.T, cfg *service.ServiceConfigs, name string) *blockingChildImpl {
	t.Helper()
	childCfg := *cfg
	childCfg.Name = name

	child := &blockingChildImpl{
		started: make(chan struct{}),
		done:    make(chan struct{}),
	}
	require.NoError(t, service.InitServiceTemplate(&childCfg, &child.ServiceTemplate, child))
	return child
}

type NodeSuite struct {
	suite.Suite
}

func TestServe(t *testing.T) {
	suite.Run(t, new(NodeSuite))
}

func (s *NodeSuite) TestNodeStopCancelsChildContexts() {
	ctx, cancel := context.WithCancel(context.Background())
	parentCfg := service.ServiceConfigs{
		Name:     "node",
		LogLevel: slog.LevelError,
		Context:  ctx,
		Cancel:   cancel,
	}

	nodeSvc := &Service{}
	require.NoError(s.T(), service.InitServiceTemplate(&parentCfg, &nodeSvc.ServiceTemplate, nodeSvc))

	child1 := createBlockingChild(s.T(), &parentCfg, "child-1")
	child2 := createBlockingChild(s.T(), &parentCfg, "child-2")
	nodeSvc.Children = []service.IService{child1, child2}

	done := make(chan struct{})
	go func() {
		_ = nodeSvc.Serve()
		close(done)
	}()

	select {
	case <-child1.started:
	case <-time.After(2 * time.Second):
		s.Fail("child-1 did not start")
	}
	select {
	case <-child2.started:
	case <-time.After(2 * time.Second):
		s.Fail("child-2 did not start")
	}

	nodeSvc.Stop(false)

	select {
	case <-child1.done:
	case <-time.After(2 * time.Second):
		s.Fail("child-1 did not observe ctx.Done()")
	}
	select {
	case <-child2.done:
	case <-time.After(2 * time.Second):
		s.Fail("child-2 did not observe ctx.Done()")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		s.Fail("node did not exit after Stop()")
	}
}

type errorChildImpl struct {
	service.ServiceTemplate
	started chan struct{}
}

func (c *errorChildImpl) OnServe(ctx context.Context) error {
	close(c.started)
	time.Sleep(10 * time.Millisecond)
	return fmt.Errorf("Oops %s!", c.Name)
}

func createErrorChild(t *testing.T, cfg *service.ServiceConfigs, name string) *errorChildImpl {
	t.Helper()
	childCfg := *cfg
	childCfg.Name = name

	child := &errorChildImpl{
		started: make(chan struct{}),
	}
	require.NoError(t, service.InitServiceTemplate(&childCfg, &child.ServiceTemplate, child))
	return child
}

func (s *NodeSuite) TestNodeReturnChildErrors() {

	ctx, cancel := context.WithCancel(context.Background())
	parentCfg := service.ServiceConfigs{
		Name:     "node",
		LogLevel: slog.LevelError,
		Context:  ctx,
		Cancel:   cancel,
	}

	nodeSvc := &Service{}
	require.NoError(s.T(), service.InitServiceTemplate(&parentCfg, &nodeSvc.ServiceTemplate, nodeSvc))

	child1 := createErrorChild(s.T(), &parentCfg, "child-1")
	child2 := createErrorChild(s.T(), &parentCfg, "child-2")
	nodeSvc.Children = []service.IService{child1, child2}

	done := make(chan error)
	go func() {
		err := nodeSvc.Serve()
		done <- err
		close(done)
	}()

	select {
	case <-child1.started:
	case <-time.After(2 * time.Second):
		s.Fail("child-1 did not start")
	}
	select {
	case <-child2.started:
	case <-time.After(2 * time.Second):
		s.Fail("child-2 did not start")
	}

	select {
	case err := <-done:
		s.ErrorContains(err, "Oops child-1!")
		s.ErrorContains(err, "Oops child-2!")
	case <-time.After(2 * time.Second):
		s.Fail("node did not exit after child errors")
	}
}
