package main

import (
	"bufio"
	"io"
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

func inputMain(infLog, errLog *log.Logger, done done.Done, tunDev *os.File, config *config.Config) error {
	if config.Input.Sock == "" {
		return errors.New("input sock cannot be empty")
	}

	if config.Input.Filter == "" {
		return errors.New("input filter cannot be empty")
	}

	_, filter, err := net.ParseCIDR(config.Input.Filter)
	if err != nil {
		return errors.Wrap(err)
	}
	if len(filter.IP) != 4 {
		return errors.New("filter is not IPv4:" + filter.IP.String())
	}

	listener, err := net.Listen("unix", config.Input.Sock)
	if err != nil {
		return errors.Wrap(err)
	}
	go func() {
		<-done.WaitCh()
		os.Remove(config.Input.Sock)
		listener.Close()
	}()

	tunWriter := writerworker.New(tunDev, 0, nil)
	defer tunWriter.Close()

	wg := &sync.WaitGroup{}

	for {
		conn, err := listener.Accept()
		if done.IsDone() {
			if err == nil {
				conn.Close()
			}
			break
		}
		if err != nil {
			errLog.Println(err)
			util.RandomSleep(done)
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			inputProcessConn(infLog, errLog, done, filter, tunWriter, conn)
		}()
	}

	wg.Wait()
	return nil
}

func inputProcessConn(
	infLog, errLog *log.Logger, allDone done.Done,
	filter *net.IPNet, tunWriter *writerworker.Worker, conn net.Conn,
) {
	defer conn.Close()

	connDone := done.New()
	defer connDone.Done()

	go func() {
		select {
		case <-allDone.WaitCh():
			conn.SetReadDeadline(time.Unix(0, 0))
		case <-connDone.WaitCh():
		}
	}()

	buffConn := bufio.NewReaderSize(conn, 1<<16)

	for {
		buff, buffConsumed := buffer.Get(), false
		shouldBreak := func() bool {
			var err error
			buff.Len, err = io.ReadFull(buffConn, buff.Data[:20])
			if allDone.IsDone() {
				return true
			}
			if err == io.EOF {
				return true
			}
			if err != nil {
				errLog.Println(err)
				return true
			}

			if util.IPVersion(buff) != 4 {
				errLog.Println(errors.New("got non ipv4 packet"))
				return true
			}

			buff.Len = util.IPv4Len(buff)

			_, err = io.ReadFull(buffConn, buff.Data[20:buff.Len])
			if allDone.IsDone() {
				return true
			}
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			if err != nil {
				errLog.Println(err)
				return true
			}

			if !filter.Contains(util.IPv4Dst(buff)) {
				return false
			}

			buffConsumed = true
			tunWriter.Write(buff)
			return false
		}()
		if !buffConsumed {
			buffer.Put(buff)
		}
		if allDone.IsDone() || shouldBreak {
			break
		}
	}
}
