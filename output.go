package main

import (
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/payfazz/go-errors"

	"github.com/payfazz/socktunrouter/internal/buffer"
	"github.com/payfazz/socktunrouter/internal/config"
	"github.com/payfazz/socktunrouter/internal/done"
	"github.com/payfazz/socktunrouter/internal/util"
	"github.com/payfazz/socktunrouter/internal/writerworker"
)

func outputMain(infLog, errLog *log.Logger, done done.Done, tunDev *os.File, config *config.Config) error {
	outputRouter := newOutputRouter(done)

	for _, item := range config.Output {
		if item.Sock == "" {
			return errors.New("output sock cannot be empty")
		}

		if item.Filter == "" {
			return errors.New("output filter cannot be empty")
		}

		_, filter, err := net.ParseCIDR(item.Filter)
		if err != nil {
			return errors.Wrap(err)
		}
		if len(filter.IP) != 4 {
			return errors.New("filter is not IPv4:" + filter.IP.String())
		}

		outputRouter.router = append(outputRouter.router, newOutputWriter(errLog, filter, item.Sock, done))
	}

	go func() {
		<-done.WaitCh()
		tunDev.SetReadDeadline(time.Unix(0, 0))
	}()

	for {
		buff, buffConsumed := buffer.Get(), false
		shouldBreak := func() bool {
			var err error
			buff.Len, err = tunDev.Read(buff.Data[:])
			if done.IsDone() {
				return true
			}
			if err != nil {
				errLog.Println(err)
				util.RandomSleep(done)
				return false
			}

			if util.IPVersion(buff) != 4 {
				return false
			}

			dst := util.IPv4Dst(buff)
			outputWriter := outputRouter.getWriter(dst)
			if outputWriter == nil {
				return false
			}

			buffConsumed = true
			outputWriter.write(buff)
			return false
		}()
		if !buffConsumed {
			buffer.Put(buff)
		}
		if done.IsDone() || shouldBreak {
			break
		}
	}

	return nil
}

type dummyWriterType struct{}

func (dummyWriterType) Write(p []byte) (int, error) { return len(p), nil }

func (dummyWriterType) SetWriteDeadline(time.Time) error { return nil }

var dummyWriter = writerworker.New(dummyWriterType{}, 0, nil)

type outputWriter struct {
	sync.RWMutex
	filter *net.IPNet
	sock   string
	errLog *log.Logger
	done   done.Done
	worker *writerworker.Worker
}

func newOutputWriter(errLog *log.Logger, filter *net.IPNet, sock string, done done.Done) *outputWriter {
	return &outputWriter{filter: filter, sock: sock, errLog: errLog, done: done}
}

func (o *outputWriter) getWorker() *writerworker.Worker {
	o.RLock()
	w := o.worker
	o.RUnlock()
	return w
}

func (o *outputWriter) setWorker(w *writerworker.Worker) {
	o.Lock()
	o.worker = w
	o.Unlock()
}

func (o *outputWriter) write(buff *buffer.Buff) {
	w := o.getWorker()

	if w != nil {
		w.Write(buff)
		return
	}

	// discard buff
	buffer.Put(buff)

	if o.done.IsDone() {
		return
	}

	o.setWorker(dummyWriter)

	go func() {
		conn, err := net.Dial("unix", o.sock)
		if err != nil {
			o.errLog.Println(err)
			util.RandomSleep(o.done)
			o.setWorker(nil)
			return
		}

		errCh := make(chan error, 1)
		w := writerworker.New(conn, 0, errCh)
		go func() {
			select {
			case err := <-errCh:
				o.errLog.Println(err)
			case <-o.done.WaitCh():
			}
			o.setWorker(nil)
			w.Close()
			conn.Close()
		}()

		o.setWorker(w)
	}()
}

type outputRouter struct {
	sync.RWMutex
	router []*outputWriter
	data   map[uint32]*outputWriter
	done   done.Done
}

func newOutputRouter(done done.Done) *outputRouter {
	return &outputRouter{
		data: make(map[uint32]*outputWriter),
		done: done,
	}
}

func (o *outputRouter) getWriter(ip net.IP) *outputWriter {
	ipnumber := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])<<0

	var ret *outputWriter

	o.RLock()
	ret, ok := o.data[ipnumber]
	o.RUnlock()
	if ok {
		return ret
	}

	ret = nil
	for _, r := range o.router {
		if r.filter.Contains(ip) {
			ret = r
			break
		}
	}

	o.Lock()
	o.data[ipnumber] = ret
	o.Unlock()
	go func() {
		util.Sleep(5*time.Minute, o.done)
		o.Lock()
		delete(o.data, ipnumber)
		o.Unlock()
	}()

	return ret
}
