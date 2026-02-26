// Package progress provides terminal progress indicators.
package progress

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/rybkr/gitvista/internal/termcolor"
)

// Spinner displays an animated braille spinner on stderr while a long-running
// operation is in progress. It is only displayed when stderr is a TTY;
// in non-interactive environments (piped output, CI, E2E tests) it is silent.
type Spinner struct {
	msg  string
	done chan struct{}
	wg   sync.WaitGroup
}

// New creates a Spinner that will display msg alongside the animation.
func New(msg string) *Spinner {
	return &Spinner{
		msg:  msg,
		done: make(chan struct{}),
	}
}

// Start begins the spinner animation in a background goroutine.
// It writes to stderr so it never pollutes stdout.
func (s *Spinner) Start() {
	if !termcolor.IsTerminal(os.Stderr.Fd()) {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-s.done:
				// Clear the spinner line.
				fmt.Fprintf(os.Stderr, "\r\033[K")
				return
			case <-ticker.C:
				fmt.Fprintf(os.Stderr, "\r%s %s", frames[i%len(frames)], s.msg)
				i++
			}
		}
	}()
}

// Stop halts the spinner animation and clears the line.
func (s *Spinner) Stop() {
	select {
	case <-s.done:
		// Already stopped.
	default:
		close(s.done)
	}
	s.wg.Wait()
}
