package pool

import (
	"context"
	"log"
	"sync"
)

// Task 任务接口
type Task interface {
	Execute() error
}

// Pool 协程池
type Pool struct {
	workerNum int
	taskChan  chan Task
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewPool 创建协程池
// workerNum: 工作协程数量
// bufferSize: 任务通道缓冲区大小
func NewPool(workerNum int, bufferSize int) *Pool {
	ctx, cancel := context.WithCancel(context.Background())
	return &Pool{
		workerNum: workerNum,
		taskChan:  make(chan Task, bufferSize),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start 启动工作协程
func (p *Pool) Start() {
	for i := 0; i < p.workerNum; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
	log.Printf("[POOL] Started %d workers", p.workerNum)
}

// worker 工作协程
func (p *Pool) worker(id int) {
	defer p.wg.Done()
	for {
		select {
		case <-p.ctx.Done():
			log.Printf("[POOL] Worker %d stopped", id)
			return
		case task, ok := <-p.taskChan:
			if !ok {
				log.Printf("[POOL] Worker %d channel closed", id)
				return
			}
			if err := task.Execute(); err != nil {
				log.Printf("[POOL] Worker %d execute task failed: %v", id, err)
			}
		}
	}
}

// Submit 提交任务
// 如果通道已满，会丢弃任务（防止阻塞主流程）
func (p *Pool) Submit(task Task) {
	select {
	case p.taskChan <- task:
		// 提交成功
	default:
		// 队列满了，丢弃任务
		log.Printf("[POOL] Task queue is full, dropping task")
	}
}

// Stop 停止协程池
func (p *Pool) Stop() {
	p.cancel()
	close(p.taskChan)
	p.wg.Wait()
	log.Printf("[POOL] Pool stopped")
}
