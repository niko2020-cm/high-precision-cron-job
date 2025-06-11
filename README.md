# 高精度定时任务管理系统

## 项目简介
本项目是一个基于 Go 语言的高精度定时任务管理系统，使用 `github.com/robfig/cron/v3` 库实现定时任务的调度和管理。系统支持通过函数或接口添加定时任务，同时支持一次性任务，并且会自动清理空闲的 `cron` 实例以节省资源。

## 功能特性
- **任务管理**：支持通过函数或接口添加定时任务，可查询和删除任务。
- **一次性任务**：支持只执行一次的任务，任务执行完成后自动移除。
- **资源管理**：保留 2 个核心 `cron` 实例，动态创建和销毁 `cron` 实例，自动清理 2 小时未使用的空闲实例。
- **状态管理**：每个 `cron` 实例有空闲、忙碌和已移除三种状态。

## 依赖项
本项目依赖于 `github.com/robfig/cron/v3` 库，可通过以下命令安装：
```sh
go get github.com/robfig/cron/v3
```

## 项目结构
```
.idea/
  .gitignore
  MarsCodeWorkspaceAppSettings.xml
  high-precision-cron-job.iml
  material_theme_project_new.xml
  modules.xml
  vcs.xml
  workspace.xml
go.mod
go.sum
main.go
```

## 使用方法
### 1. 创建 `TaskTimer` 实例
```go
import "your_package_name/timer"

func main() {
    taskTimer := timer.NewTaskTimer()
    defer taskTimer.Close()
    // 后续操作...
}
```

### 2. 添加定时任务
#### 通过函数添加任务
```go
// 定义任务函数
task := func() {
    // 任务逻辑
    println("Task executed!")
}

// 添加任务
taskId, err := taskTimer.AddTaskByFunc("my_task", "* * * * *", task)
if err != nil {
    panic(err)
}
```

#### 添加一次性任务
```go
// 定义任务函数
task := func() {
    // 任务逻辑
    println("One-time task executed!")
}

// 添加一次性任务
taskId, err := taskTimer.OnceTask("one_time_task", "@once", task)
if err != nil {
    panic(err)
}
```

### 3. 查询任务
```go
exists := taskTimer.FindTask("my_task")
if exists {
    println("Task exists!")
}
```

### 4. 删除任务
```go
 taskTimer.Remove("my_task")
```

## API 文档
### `TaskTimer` 结构体
```go
type TaskTimer struct {
    taskList    map[string]contextKey
    coreCron    [2]*cronManager
    mu          sync.Mutex
    dynamicCron []*cronManager
    stopCheck   chan struct{}
    checkWg     sync.WaitGroup
}
```

### 方法
- `NewTaskTimer() *TaskTimer`：创建一个新的 `TaskTimer` 实例。
- `AddTaskByFunc(taskName string, spec string, task func(), option ...cron.Option) (cron.EntryID, error)`：通过函数添加定时任务。
- `OnceTask(taskName string, spec string, task func(), option ...cron.Option) (cron.EntryID, error)`：添加一次性任务。
- `AddTaskByJob(taskName string, spec string, job interface{ Run() }, option ...cron.Option) (cron.EntryID, error)`：通过接口添加定时任务。
- `FindTask(taskName string) bool`：查询任务是否存在。
- `Remove(taskName string)`：删除任务。
- `Close()`：释放所有资源。

## 注意事项
- 调用 `Close()` 方法后，`TaskTimer` 实例将无法再使用，需要重新创建。
- 任务调度规则遵循 `github.com/robfig/cron/v3` 库的规则。

## 贡献
如果你想为这个项目做出贡献，请提交 Pull Request 或创建 Issue。

## 许可证
本项目采用 [MIT 许可证](LICENSE)。
