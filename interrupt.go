package main

import (
	"os"
	"os/signal"
	"syscall"
)

func interruptHandler(f func(os.Signal)) {
	go func() {
		sigs := []os.Signal{syscall.SIGTERM, syscall.SIGINT}
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, sigs...)
		sig := <-sigCh
		signal.Reset(sigs...)
		f(sig)
	}()
}
