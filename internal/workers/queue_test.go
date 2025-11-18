package workers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/NGRsoftlab/pulsar/internal/types"
)

type mockJob struct{}

func (m *mockJob) ExecuteBatch() error { return nil }

func TestLockFreeQueue_Basic(t *testing.T) {
	q := NewLockFreeQueue(2)

	job1 := &mockJob{}
	assert.True(t, q.TryPush(job1), "ожидалось успешное добавление задачи")
	assert.Equal(t, job1, q.TryPop(), "ожидалaсь та же задача при извлечении")
	assert.Nil(t, q.TryPop(), "ожидалось nil при попытке извлечь из пустой очереди")
}

func TestLockFreeQueue_Full(t *testing.T) {
	q := NewLockFreeQueue(2) // Capacity = 2 элемента

	job1 := &mockJob{}
	job2 := &mockJob{}
	job3 := &mockJob{}

	assert.True(t, q.TryPush(job1), "первая задача должна поместиться")
	assert.True(t, q.TryPush(job2), "вторая задача должна поместиться") // ✅ Изменено на True
	assert.False(t, q.TryPush(job3), "третья задача не должна поместиться (очередь полна)")

	assert.Equal(t, job1, q.TryPop(), "должна быть извлечена первая задача")

	assert.True(t, q.TryPush(job3), "теперь можно поместить третью задачу")

	assert.Equal(t, job2, q.TryPop(), "должна быть извлечена вторая задача")
	assert.Equal(t, job3, q.TryPop(), "должна быть извлечена третья задача")
	assert.Nil(t, q.TryPop(), "очередь пуста")
}

func TestLockFreeQueue_WaitPop_Cancel(t *testing.T) {
	q := NewLockFreeQueue(1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan types.JobBatch, 1)
	go func() {
		job := q.WaitPop(ctx)
		done <- job
	}()

	cancel()

	select {
	case job := <-done:
		assert.Nil(t, job, "ожидалось nil при отмене контекста")
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("WaitPop не завершился после отмены контекста")
	}
}

func TestLockFreeQueue_WaitPop_WakeOnPush(t *testing.T) {
	q := NewLockFreeQueue(2)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	done := make(chan types.JobBatch, 1)
	go func() {
		job := q.WaitPop(ctx)
		done <- job
	}()

	time.Sleep(10 * time.Millisecond)

	job1 := &mockJob{}
	assert.True(t, q.TryPush(job1), "ожидалось успешное добавление задачи")

	select {
	case job := <-done:
		assert.Equal(t, job1, job, "ожидалась отправленная задача")
	case <-ctx.Done():
		t.Fatalf("WaitPop не вернул задачу после TryPush")
	}
}
