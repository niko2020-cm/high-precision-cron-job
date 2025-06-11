package timer

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

const (
	// IdleStatus 任务状态
	IdleStatus    = "idle"
	BusyStatus    = "busy"
	RemovedStatus = "removed"
)

// cronManager 管理每个任务名对应的cron实例和其下的任务ID
type cronManager struct {
	cronInst *cron.Cron
	status   string // "idle" or "busy" or "removing"
	option   []cron.Option
	lastUsed time.Time
	mu       sync.Mutex // 保护status
}

func newCronManager(option ...cron.Option) *cronManager {
	timerWorker := cron.New(option...)
	timerWorker.Start()
	return &cronManager{
		cronInst: timerWorker,
		status:   IdleStatus, // 初始状态为空闲
	}
}

func (m *cronManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cronInst.Stop()
	m.status = RemovedStatus // 标记已经被移除
}

func (m *cronManager) checkAlive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.cronInst.Entries()) == 0
}

func (m *cronManager) checkIdle() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status == IdleStatus
}

type contextKey struct {
	*cronManager
	cron.EntryID
}

// TaskTimer 定时任务管理实现
type TaskTimer struct {
	taskList    map[string]contextKey
	coreCron    [2]*cronManager // 保留2个核心cron 无option
	mu          sync.Mutex      // 保护taskList和cron列表
	dynamicCron []*cronManager  // 用于存储动态的cron实例
	stopCheck   chan struct{}
	checkWg     sync.WaitGroup
}

// NewTaskTimer 创建一个新的 taskTimer 实例
func NewTaskTimer() *TaskTimer {
	t := &TaskTimer{
		taskList:  make(map[string]contextKey, 10),
		stopCheck: make(chan struct{}),
	}
	// 初始化核心cron
	t.coreCron[0] = newCronManager()
	t.coreCron[1] = newCronManager()

	// 启动空闲cron检查协程
	t.checkWg.Add(1)
	go t.runIdleCheck()

	return t
}

// 使用预占 和 不使用释放预占位
func (t *TaskTimer) getAliveCron(option ...cron.Option) *cronManager {

	var insMgr *cronManager // 实际使用的cron实例

	// 不存在option 找空闲核心cron
	if option == nil {
		for _, mgr := range t.coreCron {
			if mgr.checkIdle() {
				insMgr = mgr
				break
			}
		}
		// 如果没有空闲核心cron，查找动态cron
		if insMgr == nil {
			for _, mgr := range t.dynamicCron {
				if mgr.checkIdle() && mgr.option == nil {
					insMgr = mgr
				}
			}
		}
	} else {
		for _, mgr := range t.dynamicCron {

			if mgr.checkIdle() && mgr.option != nil && len(mgr.option) == len(option) {
				count := 0                   // 每次判断一个cron的时候都重置成0
				for _, opt := range option { // 标记option是不是都相同
					for _, mgrOpt := range mgr.option {
						if reflect.ValueOf(opt).Pointer() != reflect.ValueOf(mgrOpt).Pointer() {
							count++
						}
					}
				}

				if count == len(option) {
					insMgr = mgr
					break
				}

			}
		}
	}

	if insMgr == nil {
		insMgr = newCronManager(option...)
		t.dynamicCron = append(t.dynamicCron, insMgr)
	}

	return insMgr

}

// AddTaskByFunc 通过函数的方法添加任务
func (t *TaskTimer) AddTaskByFunc(taskName string, spec string, task func(), option ...cron.Option) (cron.EntryID, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.taskList[taskName]
	if !ok {
		mgr := t.getAliveCron(option...)
		taskId, err := mgr.cronInst.AddFunc(spec, task)
		if err != nil {
			return 0, err
		}
		t.taskList[taskName] = contextKey{
			cronManager: mgr,
			EntryID:     taskId,
		}
		if len(mgr.cronInst.Entries()) >= 20 {
			mgr.status = BusyStatus
		}
		return taskId, nil
	}
	return t.taskList[taskName].EntryID, errors.New("任务已经启动")
}

// OnceTask 一次性任务 只执行一次 执行完成之后 就会被移除
func (t *TaskTimer) OnceTask(taskName string, spec string, task func(), option ...cron.Option) (cron.EntryID,
	error) {
	// 对提供的func 进行包装 自带一个 remove 方法
	newTask := func() {
		task()
		time.Sleep(3 * time.Second)
		t.Remove(taskName)
	}
	return t.AddTaskByFunc(taskName, spec, newTask, option...)
}

// AddTaskByJob 通过接口的方法添加任务
func (t *TaskTimer) AddTaskByJob(taskName string, spec string, job interface{ Run() }, option ...cron.Option) (cron.EntryID, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.taskList[taskName]
	if !ok {
		mgr := t.getAliveCron(option...)
		taskId, err := mgr.cronInst.AddJob(spec, job)
		if err != nil {
			return 0, err
		}
		t.taskList[taskName] = contextKey{
			cronManager: mgr,
			EntryID:     taskId,
		}
		if len(mgr.cronInst.Entries()) >= 20 {
			mgr.status = BusyStatus
		}
		return taskId, nil
	}
	return t.taskList[taskName].EntryID, errors.New("任务已经启动")
}

func (t *TaskTimer) FindTask(taskName string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.taskList[taskName]
	return ok
}

// Remove 清理任务实际上就是删除任务
func (t *TaskTimer) Remove(taskName string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	mgr, ok := t.taskList[taskName]
	if ok {
		mgr.mu.Lock()
		defer mgr.mu.Unlock()
		mgr.cronInst.Remove(mgr.EntryID)
		delete(t.taskList, taskName)
		if len(mgr.cronInst.Entries()) < 20 && mgr.status == BusyStatus {
			mgr.status = IdleStatus
		}
	}
}

// Close 释放所有资源 资源释放之后 再使用 需要通过 new 重新创建
func (t *TaskTimer) Close() {
	// 停止空闲检查协程
	close(t.stopCheck)
	t.checkWg.Wait()

	t.mu.Lock()
	defer t.mu.Unlock()

	t.taskList = nil // 将任务队列置为空

	for _, mgr := range t.coreCron {
		mgr.Stop()
	}
	for _, mgr := range t.dynamicCron {
		mgr.Stop()
	}
	t.dynamicCron = nil

	t.coreCron[0] = nil
	t.coreCron[1] = nil
}

// runIdleCheck 定期检查空闲的cron实例并销毁
func (t *TaskTimer) runIdleCheck() {
	defer t.checkWg.Done()
	ticker := time.NewTicker(1 * time.Hour) // 每个小时刷新一次
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("runIdleCheck panic:", r)
			t.runIdleCheck()
		}
	}()

	defer func() {
		ticker.Stop()
		ticker = nil
	}()
	for {
		select {
		case <-ticker.C:
			t.checkIdleCron()
		case <-t.stopCheck:
			return
		}
	}
}

// checkIdleCron 检查并销毁空闲的cron实例
func (t *TaskTimer) checkIdleCron() {
	t.mu.Lock()
	defer t.mu.Unlock()

	var aliveCron []*cronManager
	for _, mgr := range t.dynamicCron {
		if mgr.checkAlive() && time.Since(mgr.lastUsed) > 2*time.Hour { // 2小时未使用则销毁
			mgr.Stop()
		} else {
			aliveCron = append(aliveCron, mgr)
		}
	}
	t.dynamicCron = aliveCron
}
