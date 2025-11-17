package coordinator

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NGRsoftlab/pulsar/internal/domain/event"
	"github.com/NGRsoftlab/pulsar/internal/pipeline/stages"
)

// --- Мок Stage ---

type mockStage struct {
	runFn func(ctx context.Context, in <-chan *stages.SerializedData, out chan<- *stages.SerializedData, ready chan<- bool) error
}

func (m *mockStage) Run(ctx context.Context, in <-chan *stages.SerializedData, out chan<- *stages.SerializedData, ready chan<- bool) error {
	if m.runFn != nil {
		return m.runFn(ctx, in, out, ready)
	}
	// По умолчанию: только сигнал готовности
	close(ready)
	return nil
}

// --- Хелперы для моков ---

func newReadyOnlyStage() *mockStage {
	return &mockStage{}
}

func newPassThroughStage() *mockStage {
	return &mockStage{
		runFn: func(ctx context.Context, in <-chan *stages.SerializedData, out chan<- *stages.SerializedData, ready chan<- bool) error {
			close(ready)
			for data := range in {
				select {
				case out <- data:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			// НЕ закрываем out! Пайплайн сделает это в Stop().
			return nil
		},
	}
}

func newStageThatWaitsForCancel() *mockStage {
	return &mockStage{
		runFn: func(ctx context.Context, in <-chan *stages.SerializedData, out chan<- *stages.SerializedData, ready chan<- bool) error {
			close(ready)
			for {
				select {
				case data, ok := <-in:
					if !ok {
						// Канал закрыт — завершаемся
						return nil
					}
					// Игнорируем данные или что-то делаем
					_ = data
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		},
	}
}

// --- Тесты ---

func TestPipeline_Lifecycle(t *testing.T) {
	p := NewPipeline(10)
	assert.Equal(t, Stopped, p.GetStatus())

	err := p.Start(context.Background())
	require.NoError(t, err)
	assert.Equal(t, Running, p.GetStatus())

	err = p.Stop()
	require.NoError(t, err)
	assert.Equal(t, Stopped, p.GetStatus())
}

func TestPipeline_AddStage_OnlyWhenStopped(t *testing.T) {
	p := NewPipeline(10)
	stage := newReadyOnlyStage()

	// Можно добавить в Stopped
	err := p.AddStage(stage)
	require.NoError(t, err)

	// Запускаем
	err = p.Start(context.Background())
	require.NoError(t, err)

	// Нельзя добавить в Running
	err = p.AddStage(stage)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot add stage")

	p.Stop()
}

func TestPipeline_Start_OnlyWhenStopped(t *testing.T) {
	p := NewPipeline(10)

	// Первый старт — OK
	err := p.Start(context.Background())
	require.NoError(t, err)

	// Повторный старт — ошибка
	err = p.Start(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot start")

	p.Stop()
}

func TestPipeline_Stop_OnlyWhenRunning(t *testing.T) {
	p := NewPipeline(10)

	// Stop в Stopped — ошибка
	err := p.Stop()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot stop")

	// Запускаем
	err = p.Start(context.Background())
	require.NoError(t, err)

	// Stop в Running — OK
	err = p.Stop()
	require.NoError(t, err)

	// Повторный Stop — ошибка
	err = p.Stop()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot stop")
}

func TestPipeline_CreateChannels(t *testing.T) {
	p := NewPipeline(5)

	// Добавим 3 стадии
	for i := 0; i < 3; i++ {
		p.AddStage(newReadyOnlyStage())
	}

	impl := p.(*pipelineImpl)
	impl.createChannels()

	assert.Len(t, impl.channels, 2) // 3 стадии → 2 канала
	for _, ch := range impl.channels {
		assert.NotNil(t, ch)
		assert.Equal(t, 5, cap(ch))
	}
}

func TestPipeline_ZeroStages(t *testing.T) {
	p := NewPipeline(10)

	err := p.Start(context.Background())
	require.NoError(t, err)
	assert.Equal(t, Running, p.GetStatus())

	err = p.Stop()
	require.NoError(t, err)
}

func TestPipeline_OneStage(t *testing.T) {
	p := NewPipeline(10)
	p.AddStage(newPassThroughStage())

	err := p.Start(context.Background())
	require.NoError(t, err)

	// Отправим событие
	input := p.(*pipelineImpl).inputChan
	go func() {
		input <- &stages.SerializedData{
			Data:              []byte("test"),
			EventType:         event.EventTypeNetflow,
			Size:              4,
			Timestamp:         time.Now(),
			EventID:           "test-1",
			SerializationMode: "json",
			Destination:       "127.0.0.1:8080",
			Protocol:          "UDP",
		}
	}()

	var result *stages.SerializedData
	select {
	case result = <-p.(*pipelineImpl).outputChan:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for output")
	}

	assert.Equal(t, []byte("test"), result.Data)
	assert.Equal(t, event.EventTypeNetflow, result.EventType)

	err = p.Stop()
	assert.NoError(t, err)
}

func TestPipeline_DataFlow_ThroughTwoStages(t *testing.T) {
	p := NewPipeline(10)

	stage1 := &mockStage{
		runFn: func(ctx context.Context, in <-chan *stages.SerializedData, out chan<- *stages.SerializedData, ready chan<- bool) error {
			close(ready)
			for data := range in {
				data.Data = append([]byte("A_"), data.Data...)
				data.Size += 2
				select {
				case out <- data:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		},
	}

	stage2 := &mockStage{
		runFn: func(ctx context.Context, in <-chan *stages.SerializedData, out chan<- *stages.SerializedData, ready chan<- bool) error {
			close(ready)
			for data := range in {
				data.Destination = "mock:9999"
				select {
				case out <- data:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		},
	}

	p.AddStage(stage1)
	p.AddStage(stage2)

	err := p.Start(context.Background())
	require.NoError(t, err)

	p.(*pipelineImpl).inputChan <- &stages.SerializedData{
		Data:              []byte("hello"),
		EventType:         event.EventTypeNetflow,
		Size:              5,
		Timestamp:         time.Now(),
		EventID:           "id-1",
		SerializationMode: "json",
		Destination:       "orig:1234",
		Protocol:          "TCP",
	}

	// Читаем результат ДО Stop()
	var result *stages.SerializedData
	select {
	case result = <-p.(*pipelineImpl).outputChan:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	assert.Equal(t, []byte("A_hello"), result.Data)
	assert.Equal(t, 7, result.Size)
	assert.Equal(t, "mock:9999", result.Destination)
	assert.Equal(t, event.EventTypeNetflow, result.EventType)

	err = p.Stop()
	require.NoError(t, err)
}

func TestPipeline_Stop_WaitGroupSafety(t *testing.T) {
	p := NewPipeline(10)
	p.AddStage(newStageThatWaitsForCancel())

	err := p.Start(context.Background())
	require.NoError(t, err)

	// Дадим время запуститься
	time.Sleep(10 * time.Millisecond)

	// Останавливаем — должно завершиться без паники
	err = p.Stop()
	require.NoError(t, err)
}

func TestPipeline_GetStatus_ConcurrencySafety(t *testing.T) {
	p := NewPipeline(10)
	p.AddStage(newReadyOnlyStage())

	err := p.Start(context.Background())
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = p.GetStatus() // безопасное чтение
		}()
	}
	wg.Wait()

	p.Stop()
}

func TestPipeline_Stage_ReturnsError(t *testing.T) {
	p := NewPipeline(10)

	errorStage := &mockStage{
		runFn: func(ctx context.Context, in <-chan *stages.SerializedData, out chan<- *stages.SerializedData, ready chan<- bool) error {
			close(ready)
			return fmt.Errorf("intentional error")
		},
	}

	p.AddStage(errorStage)

	err := p.Start(context.Background())
	require.NoError(t, err)

	// Даем время для обработки ошибки
	time.Sleep(50 * time.Millisecond)

	// Pipeline должен успешно остановиться несмотря на ошибку в stage
	err = p.Stop()
	require.NoError(t, err)
	assert.Equal(t, Stopped, p.GetStatus())
}

func TestPipeline_MultipleDataItems(t *testing.T) {
	p := NewPipeline(10)

	stage1 := &mockStage{
		runFn: func(ctx context.Context, in <-chan *stages.SerializedData, out chan<- *stages.SerializedData, ready chan<- bool) error {
			close(ready)
			count := 0
			for data := range in {
				count++
				data.Data = append([]byte{byte(count)}, data.Data...)
				select {
				case out <- data:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		},
	}

	p.AddStage(stage1)

	err := p.Start(context.Background())
	require.NoError(t, err)

	// Отправляем несколько элементов
	inputChan := p.(*pipelineImpl).inputChan
	go func() {
		for i := 0; i < 5; i++ {
			inputChan <- &stages.SerializedData{
				Data:              []byte("hello"),
				EventType:         event.EventTypeNetflow,
				Size:              5,
				Timestamp:         time.Now(),
				EventID:           "id-1",
				SerializationMode: "json",
				Destination:       "orig:1234",
				Protocol:          "TCP",
			}
		}
	}()

	// Читаем все 5 элементов
	outputChan := p.(*pipelineImpl).outputChan
	for i := 0; i < 5; i++ {
		select {
		case result := <-outputChan:
			require.NotNil(t, result)
			assert.Equal(t, byte(i+1), result.Data[0]) // Проверяем счетчик
		case <-time.After(time.Second):
			t.Fatalf("timeout on item %d", i)
		}
	}

	err = p.Stop()
	require.NoError(t, err)
}

func TestPipeline_AddStage_AfterStop(t *testing.T) {
	p := NewPipeline(10)

	p.AddStage(newReadyOnlyStage())
	p.Start(context.Background())
	p.Stop()

	// После Stop (Stopped), можно добавить еще stage
	err := p.AddStage(newReadyOnlyStage())
	require.NoError(t, err)

	assert.Len(t, p.(*pipelineImpl).stages, 2)
}

func TestPipeline_ContextCancellation_PropagatesCorrectly(t *testing.T) {
	p := NewPipeline(10)

	stageGotCanceled := false
	stage := &mockStage{
		runFn: func(ctx context.Context, in <-chan *stages.SerializedData, out chan<- *stages.SerializedData, ready chan<- bool) error {
			close(ready)
			select {
			case <-ctx.Done():
				stageGotCanceled = true
				return ctx.Err()
			case <-time.After(5 * time.Second):
				t.Error("Stage didn't receive cancel signal")
			}
			return nil
		},
	}

	p.AddStage(stage)

	ctx, cancel := context.WithCancel(context.Background())
	err := p.Start(ctx)
	require.NoError(t, err)

	// Даем время на инициализацию
	time.Sleep(10 * time.Millisecond)

	// Отменяем контекст
	cancel()

	// Даем время на обработку отмены
	time.Sleep(50 * time.Millisecond)

	err = p.Stop()
	require.NoError(t, err)

	assert.True(t, stageGotCanceled, "Stage должна была получить сигнал отмены")
}

func TestPipeline_LargeBuffer_ManyItems(t *testing.T) {
	p := NewPipeline(100)

	stage := &mockStage{
		runFn: func(ctx context.Context, in <-chan *stages.SerializedData, out chan<- *stages.SerializedData, ready chan<- bool) error {
			close(ready)
			for data := range in {
				data.Size = len(data.Data)
				select {
				case out <- data:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		},
	}

	p.AddStage(stage)

	err := p.Start(context.Background())
	require.NoError(t, err)

	itemCount := 50
	inputChan := p.(*pipelineImpl).inputChan
	outputChan := p.(*pipelineImpl).outputChan

	// Отправляем много элементов без ожидания ответов
	go func() {
		for i := 0; i < itemCount; i++ {
			inputChan <- &stages.SerializedData{
				Data:        []byte(fmt.Sprintf("item-%d", i)),
				EventID:     fmt.Sprintf("id-%d", i),
				EventType:   event.EventTypeNetflow,
				Timestamp:   time.Now(),
				Destination: "test:1234",
				Protocol:    "TCP",
			}
		}
	}()

	// Читаем все элементы
	receivedCount := 0
	timeout := time.After(2 * time.Second)
	for receivedCount < itemCount {
		select {
		case result := <-outputChan:
			if result != nil {
				receivedCount++
			}
		case <-timeout:
			t.Fatalf("timeout, received %d/%d items", receivedCount, itemCount)
		}
	}

	assert.Equal(t, itemCount, receivedCount)

	err = p.Stop()
	require.NoError(t, err)
}

func TestPipeline_StageReadySignalHandling(t *testing.T) {
	p := NewPipeline(10)

	var readyMu sync.Mutex
	readyReceived := 0

	makeStage := func() *mockStage {
		return &mockStage{
			runFn: func(ctx context.Context, in <-chan *stages.SerializedData, out chan<- *stages.SerializedData, ready chan<- bool) error {
				close(ready)
				readyMu.Lock()
				readyReceived++
				readyMu.Unlock()
				return nil
			},
		}
	}

	p.AddStage(makeStage())
	p.AddStage(makeStage())

	err := p.Start(context.Background())
	require.NoError(t, err)

	// Ждём, пока обе стадии не завершатся (иначе readyReceived может быть не обновлён)
	// Но лучше — проверить после остановки
	err = p.Stop()
	require.NoError(t, err)

	readyMu.Lock()
	assert.Equal(t, 2, readyReceived, "Обе стадии должны были отправить сигнал готовности")
	readyMu.Unlock()
}

func TestPipeline_StartWithCancelledContext(t *testing.T) {
	p := NewPipeline(10)
	p.AddStage(newReadyOnlyStage())

	// Создаем уже отмененный контекст
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Start должен успешно завершиться, но стадии должны получить отмену
	err := p.Start(ctx)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	err = p.Stop()
	require.NoError(t, err)
}
