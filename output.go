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
	outputRouter := newOutputRouter()

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

		outputRouter.add(newOutputWriter(errLog, filter, item.Sock, done))
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

func (o *outputWriter) casWorker(old, new *writerworker.Worker) bool {
	var ret bool
	o.Lock()
	if o.worker == old {
		o.worker = new
		ret = true
	} else {
		ret = false
	}
	o.Unlock()
	return ret
}

func (o *outputWriter) write(buff *buffer.Buff) {
	w := o.getWorker()

	if w != nil {
		w.Write(buff)
		return
	}

	// discard buff
	buffer.Put(buff)

	if !o.casWorker(nil, dummyWriter) {
		return
	}

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
	cache  map[uint32]*outputRouterEntry
}

type outputRouterEntry struct {
	sync.RWMutex
	value *outputWriter
	exp   time.Time
}

func (o *outputRouterEntry) expired() bool {
	o.RLock()
	ret := o.exp.Before(time.Now())
	o.RUnlock()
	return ret
}

func (o *outputRouterEntry) extend() {
	o.Lock()
	o.exp = time.Now().Add(5 * time.Minute)
	o.Unlock()
}

func newOutputRouter() *outputRouter {
	return &outputRouter{
		cache: make(map[uint32]*outputRouterEntry),
	}
}

func (o *outputRouter) add(w *outputWriter) {
	o.Lock()
	o.router = append(o.router, w)
	o.Unlock()
}

func (o *outputRouter) getWriter(ip net.IP) *outputWriter {
	ipnumber := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])<<0

	o.RLock()
	cacheLen := len(o.cache)
	ret, ok := o.cache[ipnumber]
	o.RUnlock()
	if ok {
		if ret.expired() {
			if cacheLen > 2<<10 {
				o.Lock()
				delete(o.cache, ipnumber)
				o.Unlock()
			} else {
				ret.extend()
			}
		}
		return ret.value
	}

	var w *outputWriter
	o.Lock()
	if ret, ok := o.cache[ipnumber]; ok {
		return ret.value
	}
	for _, v := range o.router {
		if v.filter.Contains(ip) {
			w = v
			break
		}
	}
	ret = &outputRouterEntry{value: w}
	ret.extend()

	o.cache[ipnumber] = ret
	o.Unlock()

	return ret.value
}
