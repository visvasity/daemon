// Copyright (c) 2025 Visvasity LLC

// Package initstatus implements helper functions to listen-for and send
// successful or unsuccessful initialization status notification from a child
// process.
package initstatus

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
)

// ReceiveFunc waits for initialization status report.
type ReceiveFunc = func(context.Context) error

// Receiver starts a temporary http server listening at the returned
// address. This temporary server accepts a single incoming POST request that
// should contain the initialization status. Returned closer will close the
// server and reports [os.ErrClosed] to the receiver if no status was reported
// yet.
func Receiver(ctx context.Context) (addrURL string, receiver ReceiveFunc, closer func()) {
	errReady := errors.New("ready")
	rctx, rcancel := context.WithCancelCause(ctx)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			rcancel(err)
			return
		}
		if len(data) != 0 {
			rcancel(errors.New(string(data)))
			return
		}
		rcancel(errReady)
	}))

	go func() {
		<-rctx.Done()
		server.Close()
	}()

	receiver = func(ctx context.Context) error {
		var err error
		select {
		case <-ctx.Done():
			err = context.Cause(ctx)
		case <-rctx.Done():
			err = context.Cause(rctx)
		}
		rcancel(err)
		if !errors.Is(err, errReady) {
			return err
		}
		return nil
	}

	return server.URL, receiver, func() { rcancel(os.ErrClosed) }
}

// Report sends initialization status at the given receiver address.
func Report(ctx context.Context, addrURL string, status error) error {
	if addrURL == "" {
		return nil
	}
	var r io.Reader
	if status != nil {
		r = strings.NewReader(status.Error())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, addrURL, r)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
