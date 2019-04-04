package done

// Done .
type Done struct {
	doneCh chan struct{}
}

// New .
func New() Done {
	return Done{make(chan struct{})}
}

// IsDone .
func (d Done) IsDone() bool {
	select {
	case <-d.doneCh:
		return true
	default:
		return false
	}
}

// Done .
func (d Done) Done() {
	if !d.IsDone() {
		close(d.doneCh)
	}
}

// WaitCh .
func (d Done) WaitCh() <-chan struct{} {
	return d.doneCh
}
