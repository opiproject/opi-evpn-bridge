// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package taskmanager manages the tasks that are created for realization of intents
package taskmanager

import (
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/common"

	// Typo
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/subscriberframework/eventbus"
)

// TaskMan holds a TaskManager object
var TaskMan = newTaskManager()

// TaskManager holds fields crucial for task manager functionality
type TaskManager struct {
	taskQueue      *TaskQueue
	taskStatusChan chan *TaskStatus
	replayChan     chan struct{}
}

// Task corresponds to an onject to be realized
type Task struct {
	name            string
	objectType      string
	resourceVersion string
	subIndex        int
	retryTimer      time.Duration
	subs            []*eventbus.Subscriber
}

// TaskStatus holds info related to the status that has been received
type TaskStatus struct {
	name            string
	objectType      string
	resourceVersion string
	notificationID  string
	dropTask        bool
	component       *common.Component
}

// newTaskManager return the new task manager object
func newTaskManager() *TaskManager {
	return &TaskManager{
		taskQueue:      NewTaskQueue(),
		taskStatusChan: make(chan *TaskStatus),
		replayChan:     make(chan struct{}),
	}
}

// newTask return the new task object
func newTask(name, objectType, resourceVersion string, subs []*eventbus.Subscriber) *Task {
	return &Task{
		name:            name,
		objectType:      objectType,
		resourceVersion: resourceVersion,
		subIndex:        0,
		subs:            subs,
	}
}

// newTaskStatus return the new task status object
func newTaskStatus(name, objectType, resourceVersion, notificationID string, dropTask bool, component *common.Component) *TaskStatus {
	return &TaskStatus{
		name:            name,
		objectType:      objectType,
		resourceVersion: resourceVersion,
		notificationID:  notificationID,
		dropTask:        dropTask,
		component:       component,
	}
}

// StartTaskManager starts task manager
func (t *TaskManager) StartTaskManager() {
	go t.processTasks()
	log.Println("Task Manager has started")
}

// CreateTask creates a task and adds it to the queue
func (t *TaskManager) CreateTask(name, objectType, resourceVersion string, subs []*eventbus.Subscriber) {
	task := newTask(name, objectType, resourceVersion, subs)
	// The reason that we use a go routing to enqueue the task is because we do not want the main thread to block
	// if the queue is full but only the go routine to block
	go t.taskQueue.Enqueue(task)
	log.Printf("CreateTask(): New Task has been created: %+v\n", task)
}

// StatusUpdated creates a task status and sends it for handling
func (t *TaskManager) StatusUpdated(name, objectType, resourceVersion, notificationID string, dropTask bool, component *common.Component) {
	taskStatus := newTaskStatus(name, objectType, resourceVersion, notificationID, dropTask, component)

	// Do we need to make this call happen in a goroutine in order to not make the
	// StatusUpdated function stuck in case that nobody reads what is written on the channel ?
	// Is there any case where this can happen
	// (nobody reads what is written on the channel and the StatusUpdated gets stuck) ?
	t.taskStatusChan <- taskStatus
	log.Printf("StatusUpdated(): New Task Status has been created and sent to channel: %+v\n", taskStatus)
}

// ReplayFinished notifies that the replay of objects has finished
func (t *TaskManager) ReplayFinished() {
	close(t.replayChan)
	log.Println("ReplayFinished(): Replay has finished.")
}

// processTasks processes the task
func (t *TaskManager) processTasks() {
	var taskStatus *TaskStatus

	for {
		task := t.taskQueue.Dequeue()
		log.Printf("processTasks(): Task has been dequeued for processing: %+v\n", task)

		subsToIterate := task.subs[task.subIndex:]
	loopTwo:
		for i, sub := range subsToIterate {
			// TODO: We need a newObjectData function to create the ObjectData objects
			objectData := &eventbus.ObjectData{
				Name:            task.name,
				ResourceVersion: task.resourceVersion,
				// We need this notificationID in order to tell if the status that we got
				// in the taskStatusChan corresponds to the latest notificiation that we have sent or not.
				// (e.g. Maybe you have a timeout on the subscribers and you got the notification after the timeout have passed)
				NotificationID: uuid.NewString(),
			}
			eventbus.EBus.Publish(objectData, sub)
			log.Printf("processTasks(): Notification has been sent to subscriber %+v with data %+v\n", sub, objectData)

		loopThree:
			for {
				// We have this for loop in order to assert that the taskStatus that received from the channel is related to the current task.
				// We do that by checking the notificationID
				// If not we just ignore the taskStatus that we have received and loop again.
				taskStatus = nil
				select {
				case taskStatus = <-t.taskStatusChan:

					log.Printf("processTasks(): Task Status has been received from the channel %+v\n", taskStatus)
					if taskStatus.notificationID == objectData.NotificationID {
						log.Printf("processTasks(): received notification id %+v equals the sent notification id %+v\n", taskStatus.notificationID, objectData.NotificationID)
						break loopThree
					}
					log.Printf("processTasks(): received notification id %+v doesn't equal the sent notification id %+v\n", taskStatus.notificationID, objectData.NotificationID)

				// We need a timeout in case that the subscriber doesn't update the status at all for whatever reason.
				// If that occurs then we just take a note which subscriber need to revisit and we requeue the task without any timer
				case <-time.After(30 * time.Second):
					log.Printf("processTasks(): No task status has been received in the channel from subscriber %+v. The task %+v will be requeued immediately Task Status %+v\n", sub, task, taskStatus)
					task.subIndex += i
					go t.taskQueue.Enqueue(task)
					break loopThree
				}
			}

			// This check is needed in order to move to the next task if the status channel has timed out or we need to drop the task in case that
			// the task of the object is referring to an old already updated object or the object is no longer in the database (has been deleted)
			// or a replay procedure has been requested
			if t.checkStatus(taskStatus) {
				log.Println("processTasks(): Move to the next Task in the queue")
				break loopTwo
			}

			switch taskStatus.component.CompStatus {
			case common.ComponentStatusSuccess:
				log.Printf("processTasks(): Subscriber %+v has processed the task %+v successfully\n", sub, task)
				continue loopTwo
			default:
				log.Printf("processTasks(): Subscriber %+v has not processed the task %+v successfully\n", sub, task)
				task.subIndex += i
				task.retryTimer = taskStatus.component.Timer
				log.Printf("processTasks(): The Task will be requeued after %+v\n", task.retryTimer)
				time.AfterFunc(task.retryTimer, func() {
					t.taskQueue.Enqueue(task)
				})
				break loopTwo
			}
		}
	}
}

// checkStatus checks if the taskStatus is nill or if the current Task
// should be dropped or if a replay procedure has been requested
func (t *TaskManager) checkStatus(taskStatus *TaskStatus) bool {
	if taskStatus == nil {
		return true
	}

	if taskStatus.dropTask {
		if taskStatus.component.Replay {
			log.Println("checkStatus(): Wait for the replay DB procedure to finish and move to the next Task in the queue")
			<-t.replayChan
			log.Println("checkStatus(): Replay has finished. Continuing processing tasks")
		}
		return true
	}

	return false
}
