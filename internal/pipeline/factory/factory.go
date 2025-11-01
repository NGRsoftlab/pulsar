// Package factory provides utilities to construct and configure
// the event processing pipeline from application configuration.
package factory

import (
	"fmt"
	"strings"

	"github.com/nashabanov/ueba-event-generator/internal/config"
	"github.com/nashabanov/ueba-event-generator/internal/domain/event"
	"github.com/nashabanov/ueba-event-generator/internal/pipeline/coordinator"
	"github.com/nashabanov/ueba-event-generator/internal/pipeline/stages"
)

type PipelineFactory struct {
	cfg *config.Config
}

func NewPipelineFactory(cfg *config.Config) *PipelineFactory {
	return &PipelineFactory{cfg: cfg}
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
	genStage := stages.NewEventGenerationStage(
		f.cfg.Generator.EventsPerSecond,
		f.cfg)

	eventTypes, err := f.ParseEventTypes()
	if err != nil {
		return nil, fmt.Errorf("failed to parse event types: %w", err)
	}

	if err := genStage.SetEventTypes(eventTypes); err != nil {
		return nil, fmt.Errorf("failed to set event types: %w", err)
	}

	return genStage, nil
}

func (f *PipelineFactory) createSendingStage() (coordinator.Stage, error) {
	sendStage := stages.NewNetworkSendingStage()

	if err := sendStage.SetDestinations(f.cfg.Sender.Destinations); err != nil {
		return nil, fmt.Errorf("failed to set destinations: %w", err)
	}

	if err := sendStage.SetProtocol(f.cfg.Sender.Protocol); err != nil {
		return nil, fmt.Errorf("failed to set protocol: %w", err)
	}

	return sendStage, nil
}

func (f *PipelineFactory) ParseEventTypes() ([]event.EventType, error) {
	eventTypes := make([]event.EventType, len(f.cfg.Generator.EventTypes))

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
