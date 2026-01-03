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
	tasks      chan DBTask
	db         *sql.DB
	maxRetry   int
	retryDelay time.Duration
	testMode   bool
}

func NewDBQueue(db *sql.DB) *DBQueue {
	q := &DBQueue{
		tasks:      make(chan DBTask, 100),
		db:         db,
		maxRetry:   3,
		retryDelay: 100 * time.Millisecond,
		testMode:   false,
	}
	go q.worker()
	return q
}

func NewDBQueueForTest(db *sql.DB) *DBQueue {
	q := &DBQueue{
		tasks:      make(chan DBTask, 100),
		db:         db,
		maxRetry:   3,
		retryDelay: 1 * time.Millisecond, // Minimal delay for tests
		testMode:   true,
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
		if attempt < q.maxRetry-1 { // Don't sleep after the last attempt
			if q.testMode {
				time.Sleep(q.retryDelay)
			} else {
				time.Sleep(time.Duration(attempt+1) * q.retryDelay)
			}
		}
	}
	return DBResult{Err: lastErr}
}

func (q *DBQueue) Close() {
	close(q.tasks)
}

func (q *DBQueue) DB() *sql.DB {
	return q.db
}
