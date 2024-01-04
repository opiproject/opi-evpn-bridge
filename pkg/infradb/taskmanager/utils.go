// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package taskmanager manages the tasks that are created for realization of intents
package taskmanager

// TaskQueue represents a queue of tasks
type TaskQueue struct {
	channel chan *Task
}

// NewTaskQueue initializes a TaskQueue
func NewTaskQueue() *TaskQueue {
	return &TaskQueue{
		channel: make(chan *Task, 200),
	}
}

// Enqueue push tasks into the queue
func (q *TaskQueue) Enqueue(task *Task) {
	q.channel <- task
}

// Dequeue pops task from queue
func (q *TaskQueue) Dequeue() *Task {
	return <-q.channel
}

// Close closes queue channel
func (q *TaskQueue) Close() {
	close(q.channel)
}
