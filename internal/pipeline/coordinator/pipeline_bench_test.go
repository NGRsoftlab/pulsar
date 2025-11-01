package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/nashabanov/ueba-event-generator/internal/pipeline/stages"
)

type echoStage struct{}

func (e *echoStage) Run(ctx context.Context, in <-chan *stages.SerializedData, out chan<- *stages.SerializedData, ready chan<- bool) error {
	close(ready)
	for {
		select {
		case data, ok := <-in:
			if !ok {
				return nil
			}
			select {
			case out <- data:
			case <-ctx.Done():
				return ctx.Err()
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func BenchmarkPipeline_2Stage_150KEPS(b *testing.B) {
	const (
		eventsPerRun = 150_000
		bufferSize   = 1024
	)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		p := NewPipeline(bufferSize)
		p.AddStage(&echoStage{})
		p.AddStage(&echoStage{})

		ctx, cancel := context.WithCancel(context.Background())
		if err := p.Start(ctx); err != nil {
			b.Fatalf("Failed to start pipeline: %v", err)
		}

		input := p.(*pipelineImpl).inputChan
		output := p.(*pipelineImpl).outputChan

		go func() {
			for j := 0; j < eventsPerRun; j++ {
				input <- &stages.SerializedData{
					Data:      []byte("test"),
					Timestamp: time.Now(),
					EventID:   "bench-1",
				}
			}
		}()

		for j := 0; j < eventsPerRun; j++ {
			<-output
		}

		cancel()
		if err := p.Stop(); err != nil {
			b.Fatalf("Failed to stop pipeline: %v", err)
		}
	}

	b.ReportMetric(float64(eventsPerRun), "events")
}
