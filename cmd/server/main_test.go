package main

import (
	"context"
	"testing"
	"time"
)

func TestShutdown_ResourcesWaitForServers(t *testing.T) {
	t.Parallel()

	serverStarted := make(chan struct{})
	releaseServer := make(chan struct{})
	resourceCalled := make(chan struct{}, 1)

	serverStop := func(ctx context.Context) error {
		_ = ctx
		close(serverStarted)
		<-releaseServer
		return nil
	}
	resourceStop := func(ctx context.Context) error {
		_ = ctx
		resourceCalled <- struct{}{}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		shutdown(ctx, []stopFunc{serverStop}, []stopFunc{resourceStop})
		close(done)
	}()

	<-serverStarted
	select {
	case <-resourceCalled:
		t.Fatal("resource stop called before server stop completed")
	default:
	}

	close(releaseServer)
	<-done

	select {
	case <-resourceCalled:
	default:
		t.Fatal("resource stop not called")
	}
}
