package cli

import (
	"bytes"
	"context"
	"errors"
	"github.com/raqeebshafeeque/flowd/internal/store"
	"golang.org/x/sys/unix"
	"io"
	"time"
)

func (app application) watch(ctx context.Context, opts options) error {
	wait := app.wait
	if wait == nil {
		wait = waitSecond
	}
	redraw := !opts.json && isTerminal(app.out)
	seen := false
	for {
		if ctx.Err() != nil {
			return nil
		}
		var frame bytes.Buffer
		snapshot := app
		snapshot.out = &frame
		err := snapshot.execute(ctx, opts)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if err == nil {
			if err := writeFrame(app.out, frame.Bytes(), redraw); err != nil {
				return err
			}
			seen = true
		} else if !seen || !errors.Is(err, store.ErrLocked) {
			return err
		}
		if err := wait(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
	}
}

func isTerminal(out io.Writer) bool {
	file, ok := out.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	_, err := unix.IoctlGetWinsize(int(file.Fd()), unix.TIOCGWINSZ)
	return err == nil
}

func writeFrame(out io.Writer, frame []byte, redraw bool) error {
	if redraw {
		if _, err := io.WriteString(out, "\x1b[H\x1b[2J"); err != nil {
			return err
		}
	}
	_, err := io.Copy(out, bytes.NewReader(frame))
	return err
}

func waitSecond(ctx context.Context) error {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
