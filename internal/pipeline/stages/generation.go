package stages

import (
	"context"
	"fmt"
	"log"
	"time"

	"golang.org/x/time/rate"

	"github.com/nashabanov/ueba-event-generator/internal/domain/event"
)

// EventGenerationJobBatch - пакет событий для WorkerPool
type EventGenerationJobBatch struct {
	stage  *EventGenerationStage
	out    chan<- *SerializedData
	events []event.Event
}

func NewEventGenerationJobBatch(capacity int) *EventGenerationJobBatch {
	return &EventGenerationJobBatch{
		events: make([]event.Event, 0, capacity),
	}
}

// ExecuteBatch выполняет все события в батче
func (jb *EventGenerationJobBatch) ExecuteBatch() error {
	if jb == nil {
		log.Printf("❌ CRITICAL: NetworkSendJobBatch  is nil!")
		return fmt.Errorf("job batch is nil")
	}

	for _, evt := range jb.events {
		switch e := evt.(type) {
		case *event.NetflowEvent:
			var serializedData *SerializedData
			var err error

			if jb.stage.packetMode && jb.stage.serializationMode == SerializationModeBinary {
				data, err := e.ToBinaryNetFlow()
				if err != nil {
					jb.stage.metrics.IncrementFailed()
					continue
				}
				serializedData = NewSerializedData(data, e.Type(), e.GetID(), jb.stage.serializationMode)
			} else {
				serializedData, err = NewSerializedDataFromEvent(e, jb.stage.serializationMode)
				if err != nil {
					jb.stage.metrics.IncrementFailed()
					continue
				}
			}

			select {
			case jb.out <- serializedData:
				jb.stage.metrics.IncrementGenerated()
			case <-context.Background().Done():
				return context.Canceled
			}

		default:
			// пропускаем неподдерживаемые события
			continue
		}
	}

	// очищаем батч для повторного использования
	jb.events = jb.events[:0]
	return nil
}

// EventGenerationStage генерирует события с высокой скоростью
type EventGenerationStage struct {
	eventType         event.EventType
	eventsPerSecond   int
	serializationMode SerializationMode
	packetMode        bool
	workerPool        Pool
	metrics           MetricsCollector
}

// NewEventGenerationStage создаёт стадию генерации
func NewEventGenerationStage(
	eventType event.EventType,
	eventsPerSecond int,
	serializationMode SerializationMode,
	packetMode bool,
	workerPool Pool,
	metrics MetricsCollector,
) *EventGenerationStage {
	return &EventGenerationStage{
		eventType:         eventType,
		eventsPerSecond:   eventsPerSecond,
		serializationMode: serializationMode,
		packetMode:        packetMode,
		workerPool:        workerPool,
		metrics:           metrics,
	}
}

// Run запускает генерацию событий с batch + таймаут
func (g *EventGenerationStage) Run(ctx context.Context, in <-chan *SerializedData, out chan<- *SerializedData, ready chan<- bool) error {
	limiter := rate.NewLimiter(rate.Limit(g.eventsPerSecond), 100) // burst = 100

	g.workerPool.Start(ctx)
	if ready != nil {
		close(ready)
	}
	defer g.workerPool.Stop()

	const batchSize = 50
	const batchTimeout = 10 * time.Millisecond

	var (
		currentBatch *EventGenerationJobBatch
		timer        *time.Timer
		timerC       <-chan time.Time
	)

	for {
		select {
		case <-ctx.Done():
			if currentBatch != nil && len(currentBatch.events) > 0 {
				g.workerPool.Submit(currentBatch)
			}
			return ctx.Err()

		default:
			// Rate limiting
			if err := limiter.Wait(ctx); err != nil {
				return err
			}

			evt, err := g.generateEvent()
			if err != nil {
				continue
			}

			if currentBatch == nil {
				currentBatch = &EventGenerationJobBatch{
					stage:  g,
					out:    out,
					events: make([]event.Event, 0, batchSize),
				}
				timer = time.NewTimer(batchTimeout)
				timerC = timer.C
			}

			currentBatch.events = append(currentBatch.events, evt)

			if len(currentBatch.events) >= batchSize {
				g.workerPool.Submit(currentBatch) // даже если dropped - не критично при низкой скорости
				currentBatch = nil
				if timer != nil {
					timer.Stop()
					timer = nil
					timerC = nil
				}
			}

			select {
			case <-timerC:
				if currentBatch != nil && len(currentBatch.events) > 0 {
					g.workerPool.Submit(currentBatch)
					currentBatch = nil
				}
				timer = nil
				timerC = nil
			default:
			}
		}
	}
}

// generateEvent создает одно событие Event
func (g *EventGenerationStage) generateEvent() (event.Event, error) {
	switch g.eventType {
	case event.EventTypeNetflow:
		evt := event.NewNetflowEvent()
		if err := evt.GenerateRandomTrafficData(); err != nil {
			return nil, fmt.Errorf("failed to generate random traffic data: %w", err)
		}
		return evt, nil
	default:
		return nil, fmt.Errorf("unsupported event type: %v", g.eventType)
	}
}

// Конфигурационные методы

func (g *EventGenerationStage) GetGeneratedCount() uint64 {
	generated, _, _, _ := g.metrics.GetStats()
	return generated
}

func (g *EventGenerationStage) GetFailedCount() uint64 {
	_, _, failed, _ := g.metrics.GetStats()
	return failed
}
