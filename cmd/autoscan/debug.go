package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/pprof"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	debugPortOffset        = 10000
	debugReadHeaderTimeout = 5 * time.Second
)

var listenDebug = (&net.ListenConfig{}).Listen

func debugAddress(port int) string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(port+debugPortOffset))
}

// startDebugServer binds pprof on loopback at the config port plus 10000.
// Bind and serve failures warn without stopping autoscan; a bind failure returns nil.
// The server runs until it is closed or the process exits.
func startDebugServer(port int) *http.Server {
	address := debugAddress(port)
	listener, err := listenDebug(context.Background(), "tcp4", address)
	if err != nil {
		log.Warn().Err(err).Str("addr", address).Msg("Debug Server Bind Failed")
		return nil
	}

	router := http.NewServeMux()
	router.HandleFunc("/debug/pprof/", pprof.Index)
	router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	router.HandleFunc("/debug/pprof/profile", pprof.Profile)
	router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	router.HandleFunc("/debug/pprof/trace", pprof.Trace)
	server := &http.Server{
		Addr:              listener.Addr().String(),
		Handler:           router,
		ReadHeaderTimeout: debugReadHeaderTimeout,
	}
	log.Info().Str("addr", server.Addr).Msg("Debug Server Listening")
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Warn().Err(err).Str("addr", address).Msg("Debug Server Failed")
		}
	}()
	return server
}
