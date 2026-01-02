package db

import (
	"database/sql"
	"time"
)

type DBTask struct {
	Exec func(*sql.DB) (interface{}, error)
	Resp chan DBResult
}

type DBResult struct {
	Data interface{}
	Err  error
}

type DBQueue struct {
	tasks    chan DBTask
	db       *sql.DB
	maxRetry int
}

func NewDBQueue(db *sql.DB) *DBQueue {
	q := &DBQueue{
		tasks:    make(chan DBTask, 100),
		db:       db,
		maxRetry: 3,
	}
	go q.worker()
	return q
}

func (q *DBQueue) Execute(task func(*sql.DB) (interface{}, error)) (interface{}, error) {
	resp := make(chan DBResult, 1)
	q.tasks <- DBTask{Exec: task, Resp: resp}
	result := <-resp
	return result.Data, result.Err
}

func (q *DBQueue) worker() {
	for task := range q.tasks {
		result := q.executeWithRetry(task)
		task.Resp <- result
	}
}

func (q *DBQueue) executeWithRetry(task DBTask) DBResult {
	var lastErr error
	for attempt := 0; attempt < q.maxRetry; attempt++ {
		data, err := task.Exec(q.db)
		if err == nil {
			return DBResult{Data: data, Err: nil}
		}
		lastErr = err
		time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
	}
	return DBResult{Err: lastErr}
}

func (q *DBQueue) Close() {
	close(q.tasks)
}

func (q *DBQueue) DB() *sql.DB {
	return q.db
}
