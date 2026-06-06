package protocol

type TodoStatus string

const (
	TodoPending    TodoStatus = "pending"
	TodoInProgress TodoStatus = "in_progress"
	TodoCompleted  TodoStatus = "completed"
)

func (s TodoStatus) Valid() bool {
	switch s {
	case TodoPending, TodoInProgress, TodoCompleted:
		return true
	}
	return false
}

type TodoItem struct {
	ID        string     `json:"id"`
	Content   string     `json:"content"`
	Status    TodoStatus `json:"status"`
	DependsOn []string   `json:"depends_on,omitempty"`
}

type EventTodoListUpdated struct {
	Todos []TodoItem `json:"todos"`
}
