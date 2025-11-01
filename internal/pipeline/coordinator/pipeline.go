// Package coordinator manages the pipeline lifecycle
package coordinator

import (
	"context"
	"fmt"
	"sync"

	"github.com/nashabanov/ueba-event-generator/internal/pipeline/stages"
)

// Pipeline - интерфейс для пайплайна
type Pipeline interface {
	// Lifecycle - жизненный цикл
	Start(ctx context.Context) error
	Stop() error

	// Configuration - настройка
	AddStage(stage Stage) error

	// Monitoring - мониторинг
	GetStatus() PipelineStatus
}

// Stage - интерфейс для этапа пайплайна
type Stage interface {
	// Добавляем канал ready для сигнала готовности
	Run(ctx context.Context, in <-chan *stages.SerializedData, out chan<- *stages.SerializedData, ready chan<- bool) error
}

type PipelineStatus int

const (
	Stopped PipelineStatus = iota
	Starting
	Running
	Stopping
	Error
)

// pipelineImpl - реализация пайплайна
type pipelineImpl struct {
	// Состояние
	status PipelineStatus
	mu     sync.RWMutex // защищает изменения статуса

	stages   []Stage                       // список этапов
	channels []chan *stages.SerializedData // канал между стадиями

	// Входной и выходной каналы
	inputChan  chan *stages.SerializedData
	outputChan chan *stages.SerializedData

	// Управление жизненным циклом
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Конфигурация
	bufferSize int
}

// NewPipeline создает новый экземпляр пайплайна
func NewPipeline(bufferSize int) Pipeline {
	return &pipelineImpl{
		status:     Stopped,
		stages:     make([]Stage, 0),
		channels:   make([]chan *stages.SerializedData, 0),
		inputChan:  make(chan *stages.SerializedData, bufferSize),
		outputChan: make(chan *stages.SerializedData, bufferSize),
		bufferSize: bufferSize,
	}
}

// GetStatus возвращает текущий статус пайплайна
func (p *pipelineImpl) GetStatus() PipelineStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

// Start запускает пайплайн
func (p *pipelineImpl) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.status != Stopped {
		return fmt.Errorf("cannot start, pipeline status: %v", p.status)
	}

	p.status = Starting
	p.ctx, p.cancel = context.WithCancel(ctx)

	p.createChannels()
	p.startStages()

	p.status = Running
	return nil
}

// Stop останавливает пайплайн
func (p *pipelineImpl) Stop() error {
	p.mu.Lock()
	if p.status != Running {
		p.mu.Unlock()
		return fmt.Errorf("cannot stop, pipeline status: %v", p.status)
	}
	p.status = Stopping
	p.mu.Unlock()

	if p.cancel != nil {
		p.cancel()
	}

	// Закрываем входной канал — это сигнал первой стадии о завершении
	close(p.inputChan)

	p.wg.Wait()

	p.mu.Lock()
	p.status = Stopped
	p.mu.Unlock()

	return nil
}

// AddStage добавляет новый этап в пайплайн
func (p *pipelineImpl) AddStage(stage Stage) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.status != Stopped {
		return fmt.Errorf("cannot add stage, pipeline status: %v", p.status)
	}

	p.stages = append(p.stages, stage)
	return nil
}

// createChannels создает каналы для передачи данных между этапами
func (p *pipelineImpl) createChannels() {
	stageCount := len(p.stages)
	if stageCount <= 1 {
		p.channels = nil
		return
	}

	p.channels = make([]chan *stages.SerializedData, stageCount-1)
	for i := 0; i < stageCount-1; i++ {
		p.channels[i] = make(chan *stages.SerializedData, p.bufferSize)
	}
}

// startStages запускает все этапы пайплайна
func (p *pipelineImpl) startStages() {
	// Создаём каналы готовности для каждой стадии
	readyChans := make([]chan bool, len(p.stages))
	for i := range readyChans {
		readyChans[i] = make(chan bool, 1) // буферизованный, чтобы не блокировать
	}

	// Запускаем все стадии параллельно
	for i, stage := range p.stages {
		inputChan := p.getInputChannel(i)
		outputChan := p.getOutputChannel(i)
		readyChan := readyChans[i]

		p.wg.Add(1)

		go func(s Stage, in <-chan *stages.SerializedData, out chan<- *stages.SerializedData, ready chan<- bool, index int) {
			defer p.wg.Done()
			defer close(out)

			err := s.Run(p.ctx, in, out, ready)
			if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
				fmt.Printf("Stage finished with error: %v\n", err)
			} else {
				fmt.Printf("Stage finished successfully\n")
			}
		}(stage, inputChan, outputChan, readyChan, i)
	}

	for _, ready := range readyChans {
		<-ready
	}
}

// getInputChannel возвращает входной канал для указанной стадии
func (p *pipelineImpl) getInputChannel(stageIndex int) <-chan *stages.SerializedData {
	if stageIndex == 0 {
		return p.inputChan // Первая стадия читает из входа
	}
	return p.channels[stageIndex-1] // Остальные из промежуточных
}

// getOutputChannel возвращает выходной канал для указанной стадии
func (p *pipelineImpl) getOutputChannel(stageIndex int) chan<- *stages.SerializedData {
	if stageIndex == len(p.stages)-1 {
		return p.outputChan // Последняя пишет в выход
	}
	return p.channels[stageIndex] // Остальные в промежуточные
}
