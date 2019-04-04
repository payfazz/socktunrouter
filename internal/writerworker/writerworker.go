package writerworker

import (
	"io"
	"runtime"
	"time"

	"github.com/payfazz/socktunrouter/internal/buffer"
	"github.com/payfazz/socktunrouter/internal/done"
)

// Writer .
type Writer interface {
	io.Writer
	SetWriteDeadline(time.Time) error
}

// Worker .
type Worker struct {
	w      Writer
	buffCh chan *buffer.Buff
	errCh  chan<- error
	done   done.Done
}

// New .
func New(writer Writer, backlog int, errCh chan<- error) *Worker {
	if backlog <= 0 {
		backlog = runtime.GOMAXPROCS(-1) * 4
	}
	if backlog < 4 {
		backlog = 4
	}

	w := &Worker{
		w:      writer,
		buffCh: make(chan *buffer.Buff, backlog),
		errCh:  errCh,
		done:   done.New(),
	}

	go w.run()

	return w
}

// Close .
func (w *Worker) Close() error {
	w.done.Done()
	w.w.SetWriteDeadline(time.Unix(0, 0))
	return nil
}

func (w *Worker) run() {
	for {
		select {
		case <-w.done.WaitCh():
			return
		case buff := <-w.buffCh:
			func() {
				n, err := w.w.Write(buff.Data[:buff.Len])
				if w.done.IsDone() {
					return
				}
				if err == nil && int(n) != buff.Len {
					err = io.ErrShortWrite
				}
				if err != nil {
					select {
					case w.errCh <- err:
					default:
					}
					return
				}
			}()
			buffer.Put(buff)
		}
	}
}

// Write .
// Write consume/take the ownership of buff.
// it will make sure buffer.Put is called at the end
func (w *Worker) Write(buff *buffer.Buff) {
	select {
	case w.buffCh <- buff:
	default:
		buffer.Put(buff)
	}
}
