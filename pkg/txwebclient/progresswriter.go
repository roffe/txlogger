package txwebclient

import (
	"io"
	"log"
	"time"
)

// progressWriter wraps a writer and logs progress periodically.
type progressWriter struct {
	w          io.Writer
	uploaded   int64
	lastPrint  time.Time
	printEvery time.Duration
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	if n > 0 {
		p.uploaded += int64(n)
		now := time.Now()
		if now.Sub(p.lastPrint) > p.printEvery {
			log.Printf("\ruploaded %d bytes...", p.uploaded)
			p.lastPrint = now
		}
	}
	return n, err
}
