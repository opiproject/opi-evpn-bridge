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
}

// Task corresponds to an onject to be realized
type Task struct {
	name            string
	objectType      string
	resourceVersion string
	subIndex        int
	// systemTimer is used only when we want to retry a Task due to unavailability of the Subscriber or not receiving a TaskStatus
	systemTimer time.Duration
	subs        []*eventbus.Subscriber
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
	}
}

// newTask return the new task object
func newTask(name, objectType, resourceVersion string, subs []*eventbus.Subscriber) *Task {
	return &Task{
		name:            name,
		objectType:      objectType,
		resourceVersion: resourceVersion,
		subIndex:        0,
		systemTimer:     1 * time.Second,
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
	log.Printf("StatusUpdated(): New Task Status has been created: %+v\n", taskStatus)

	// We need to have a default case here so the call is not stuck if we send to channel but there is nobody reading from the channel.
	// e.g. a subscriber got stuck and doesn't reply. The task will be requeued after the timer gets expired. In the meanwhile
	// the subscriber replies and a taskStatus is sent to channel but the call gets stuck there as the previous task has not been requeued yet
	// as the timer has not expired and the queue is empty (We assume that there is only one task in the queue).
	select {
	case t.taskStatusChan <- taskStatus:
		log.Printf("StatusUpdated(): Task Status has been sent to channel: %+v\n", taskStatus)
	default:
		log.Printf("StatusUpdated(): Task Status has not been sent to channel. Channel not available: %+v\n", taskStatus)
	}
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
			if err := eventbus.EBus.Publish(objectData, sub); err != nil {
				log.Printf("processTasks(): Failed to sent notification: %+v\n", err)
				log.Printf("processTasks(): Notification not sent to subscriber %+v with data %+v. The Task %+v will be requeued.\n", sub, objectData, task)
				// We keep this subIndex in order to know from which subscriber to start iterating after the requeue of the Task
				// so we do start again from the subscriber that returned an error or was unavailable for any reason.
				task.subIndex += i
				task.systemTimer *= 2
				log.Printf("processTasks(): The Task will be requeued after %+v\n", task.systemTimer)
				time.AfterFunc(task.systemTimer, func() {
					t.taskQueue.Enqueue(task)
				})
				break loopTwo
			}
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
				// If that occurs then we just requeue the task with a timer
				case <-time.After(30 * time.Second):
					log.Printf("processTasks(): No task status has been received in the channel from subscriber %+v. The task %+v will be requeued. Task Status %+v\n", sub, task, taskStatus)
					// We keep this subIndex in order to know from which subscriber to start iterating after the requeue of the Task
					// so we do start again from the subscriber that returned an error or was unavailable for any reason.
					task.subIndex += i
					task.systemTimer *= 2
					log.Printf("processTasks(): The Task will be requeued after %+v\n", task.systemTimer)
					time.AfterFunc(task.systemTimer, func() {
						t.taskQueue.Enqueue(task)
					})
					break loopThree
				}
			}

			// This check is needed in order to move to the next task if the status channel has timed out or we need to drop the task in case that
			// the task of the object is referring to an old already updated object or the object is no longer in the database (has been deleted).
			if taskStatus == nil || taskStatus.dropTask {
				log.Println("processTasks(): Move to the next Task in the queue")
				break loopTwo
			}

			// We re-initialize the systemTimer every time that we get a taskStatus. That means that the subscriber is available and has responded
			task.systemTimer = 1 * time.Second

			switch taskStatus.component.CompStatus {
			case common.ComponentStatusSuccess:
				log.Printf("processTasks(): Subscriber %+v has processed the task %+v successfully\n", sub, task)
				continue loopTwo
			case common.ComponentStatusError:
				log.Printf("processTasks(): Subscriber %+v has not processed the task %+v successfully\n", sub, task)
				log.Printf("processTasks(): The Task will be requeued after %+v\n", taskStatus.component.Timer)
				// We keep this subIndex in order to know from which subscriber to start iterating after the requeue of the Task
				// so we do start again from the subscriber that returned an error or was unavailable for any reason.
				task.subIndex += i
				time.AfterFunc(taskStatus.component.Timer, func() {
					t.taskQueue.Enqueue(task)
				})
				break loopTwo
			default:
				log.Printf("processTasks(): Subscriber %+v has not provided designated status for the task %+v\n", sub, task)
				log.Printf("processTasks(): The task %+v will be dropped\n", task)
				break loopTwo
			}
		}
	}
}
