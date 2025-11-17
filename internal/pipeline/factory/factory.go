// Package factory provides utilities to construct and configure
// the event processing pipeline from application configuration.
package factory

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/NGRsoftlab/pulsar/internal/config"
	"github.com/NGRsoftlab/pulsar/internal/domain/event"
	"github.com/NGRsoftlab/pulsar/internal/network"
	"github.com/NGRsoftlab/pulsar/internal/pipeline/coordinator"
	"github.com/NGRsoftlab/pulsar/internal/pipeline/stages"
	"github.com/NGRsoftlab/pulsar/internal/types"
	"github.com/NGRsoftlab/pulsar/internal/workers"
)

type PipelineFactory struct {
	cfg     *config.Config
	metrics stages.MetricsCollector
}

func NewPipelineFactory(cfg *config.Config, metrics stages.MetricsCollector) *PipelineFactory {
	return &PipelineFactory{cfg: cfg, metrics: metrics}
}

func (f *PipelineFactory) calculateBufferSize() int {
	if f.cfg.Pipeline.BufferSize != 0 {
		return f.cfg.Pipeline.BufferSize
	}
	size := f.cfg.Generator.EventsPerSecond * 3
	size = max(size, 1000)
	if size > 200000 {
		size = 200000
	}
	return size
}

func (f *PipelineFactory) CreatePipeline() (coordinator.Pipeline, error) {
	bufferSize := f.calculateBufferSize()
	pipeline := coordinator.NewPipeline(bufferSize)

	genStage, err := f.createGenerationStage()
	if err != nil {
		return nil, fmt.Errorf("failed to create generation stage: %w", err)
	}

	sendStage, err := f.createSendingStage()
	if err != nil {
		return nil, fmt.Errorf("failed to create sending stage: %w", err)
	}

	if err := pipeline.AddStage(genStage); err != nil {
		return nil, fmt.Errorf("failed to add generation stage: %w", err)
	}

	if err := pipeline.AddStage(sendStage); err != nil {
		return nil, fmt.Errorf("failed to add sending stage: %w", err)
	}

	return pipeline, nil
}

func (f *PipelineFactory) createGenerationStage() (coordinator.Stage, error) {
	// 1. Обрабатываем конфигурацию НА УРОВНЕ ФАБРИКИ
	eventType, err := f.parseSingleEventType()
	if err != nil {
		return nil, err
	}

	packetMode := f.cfg.Generator.PacketMode

	// 2. Определяем режим рериализации
	var serializationMode stages.SerializationMode
	switch eventType {
	case event.EventTypeNetflow:
		serializationMode = stages.SerializationModeBinary
	case event.EventTypeSyslog:
		serializationMode = stages.SerializationModeRaw
	default:
		return nil, fmt.Errorf("unsupported event type for generation: %v", eventType)
	}

	// 3. Создаём пул воркеров
	queueSize := max(f.cfg.Generator.EventsPerSecond*2, 1000)
	workerPool := workers.NewWorkerPool(0, queueSize, func() types.JobBatch {
		return stages.NewEventGenerationJobBatch(50)
	}, f.metrics.(workers.WorkerMetrics))
	workerPool.SetPoolType("generation")

	// 4. Создаём этап с ЧИСТЫМ ИНТЕРФЕЙСОМ
	return stages.NewEventGenerationStage(
		eventType,
		f.cfg.Generator.EventsPerSecond,
		serializationMode,
		packetMode,
		workerPool,
		f.metrics,
	), nil
}

func (f *PipelineFactory) parseSingleEventType() (event.Type, error) {
	if len(f.cfg.Generator.EventTypes) == 0 {
		return 0, fmt.Errorf("at least one event type required")
	}

	if len(f.cfg.Generator.EventTypes) > 1 {
		log.Printf("⚠️ Multiple event types configured, using first: %s",
			f.cfg.Generator.EventTypes[0])
	}

	switch strings.ToLower(f.cfg.Generator.EventTypes[0]) {
	case "netflow":
		return event.EventTypeNetflow, nil
	case "syslog":
		return event.EventTypeSyslog, nil
	default:
		return 0, fmt.Errorf("unsupported event type: %s", f.cfg.Generator.EventTypes[0])
	}
}

func (f *PipelineFactory) createSendingStage() (coordinator.Stage, error) {
	if len(f.cfg.Sender.Destinations) == 0 {
		return nil, fmt.Errorf("destinations cannot be empty")
	}

	sender, err := f.createSender(f.cfg.Sender.Destinations[0], f.cfg.Sender.Protocol)
	if err != nil {
		return nil, fmt.Errorf("failed to create sender: %w", err)
	}

	wm := f.metrics.(workers.WorkerMetrics)

	workerPool := workers.NewWorkerPool(
		0,
		5000,
		func() types.JobBatch {
			return stages.NewNetworkSendJobBatch(50)
		},
		wm,
	)
	workerPool.SetPoolType("network")

	sendStage := stages.NewNetworkSendingStage(f.metrics, workerPool, sender)

	if err := sendStage.SetDestinations(f.cfg.Sender.Destinations); err != nil {
		return nil, fmt.Errorf("failed to set destinations: %w", err)
	}

	if err := sendStage.SetProtocol(f.cfg.Sender.Protocol); err != nil {
		return nil, fmt.Errorf("failed to set protocol: %w", err)
	}

	return sendStage, nil
}

func (f *PipelineFactory) createSender(dest, protocol string) (stages.Sender, error) {
	timeout := 5 * time.Second
	switch strings.ToLower(protocol) {
	case "udp":
		return network.NewUDPSender(dest, timeout)
	case "tcp":
		return network.NewTCPSender(dest, 12, timeout)
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}
}

func (f *PipelineFactory) ParseEventTypes() ([]event.Type, error) {
	eventTypes := make([]event.Type, len(f.cfg.Generator.EventTypes))

	for i, typeStr := range f.cfg.Generator.EventTypes {
		switch strings.ToLower(typeStr) {
		case "netflow":
			eventTypes[i] = event.EventTypeNetflow
		case "syslog":
			return nil, fmt.Errorf("syslog events are not supported yet")
		default:
			return nil, fmt.Errorf("unsupported event type: %s", typeStr)
		}
	}

	return eventTypes, nil
}
