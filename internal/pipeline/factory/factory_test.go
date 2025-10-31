package factory

import (
	"testing"

	"github.com/nashabanov/ueba-event-generator/internal/config"
	"github.com/nashabanov/ueba-event-generator/internal/domain/event"
	"github.com/stretchr/testify/assert"
)

func newTestConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Generator.EventTypes = []string{"netflow", "syslog"}
	cfg.Generator.EventsPerSecond = 100
	cfg.Sender.Destinations = []string{"127.0.0.1:514"}
	cfg.Sender.Protocol = "udp"
	cfg.Pipeline.BufferSize = 10
	return cfg
}

func TestParseEventTypes_Success(t *testing.T) {
	cfg := newTestConfig()
	f := NewPipelineFactory(cfg)

	types, err := f.ParseEventTypes()
	assert.NoError(t, err)
	assert.Len(t, types, 2)
	assert.Contains(t, types, event.EventTypeNetflow)
	assert.Contains(t, types, event.EventTypeSyslog)
}

func TestParseEventTypes_Unsupported(t *testing.T) {
	cfg := newTestConfig()
	cfg.Generator.EventTypes = []string{"unknown"}
	f := NewPipelineFactory(cfg)

	types, err := f.ParseEventTypes()
	assert.Error(t, err)
	assert.Nil(t, types)
}

func TestCreateGenerationStage(t *testing.T) {
	cfg := newTestConfig()
	f := NewPipelineFactory(cfg)

	stage, err := f.createGenerationStage()
	assert.NoError(t, err)
	assert.NotNil(t, stage)
}

func TestCreateSendingStage(t *testing.T) {
	cfg := newTestConfig()
	f := NewPipelineFactory(cfg)

	stage, err := f.createSendingStage()
	assert.NoError(t, err)
	assert.NotNil(t, stage)
}

func TestCreatePipeline_Success(t *testing.T) {
	cfg := newTestConfig()
	f := NewPipelineFactory(cfg)

	pipeline, err := f.CreatePipeline()
	assert.NoError(t, err)
	assert.NotNil(t, pipeline)
}

func TestCreatePipeline_InvalidEventType(t *testing.T) {
	cfg := newTestConfig()
	cfg.Generator.EventTypes = []string{"netflow", "invalid"}
	f := NewPipelineFactory(cfg)

	pipeline, err := f.CreatePipeline()
	assert.Error(t, err)
	assert.Nil(t, pipeline)
}
