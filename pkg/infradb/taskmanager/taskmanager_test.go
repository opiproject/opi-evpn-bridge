package taskmanager

import (
	"log"
	"sync"
	"testing"
	"time"

	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/common"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/subscriberframework/eventbus"
	"github.com/stretchr/testify/assert"
)

var (
	retValMu sync.Mutex
	retVal   bool
)

type moduleCiHandler struct {
	receivedEvents []*eventbus.ObjectData
}

func handleTestEvent(objectData *eventbus.ObjectData) {
	name := "testTask"
	objectType := "testType"
	resourceVersion := "testVersion"
	dropTask := false

	component := &common.Component{
		CompStatus: common.ComponentStatusSuccess,
	}

	TaskMan.StatusUpdated(name, objectType, resourceVersion, objectData.NotificationID, dropTask, component)
}
func handleTestEventCompSuccess(objectData *eventbus.ObjectData) {
	name := "testTaskCompSuccess"
	objectType := "testTypeCompSuccess"
	resourceVersion := "testVersionCompSuccess"
	dropTask := false

	component := &common.Component{
		CompStatus: common.ComponentStatusSuccess,
	}

	TaskMan.StatusUpdated(name, objectType, resourceVersion, objectData.NotificationID, dropTask, component)
}
func handleTestEventbusy(wg *sync.WaitGroup) {
	name := "testTaskbusy"
	objectType := "testTypebusy"
	resourceVersion := "testVersionbusy"
	dropTask := false

	component := &common.Component{
		CompStatus: common.ComponentStatusSuccess,
	}

	TaskMan.StatusUpdated(name, objectType, resourceVersion, "notificationID", dropTask, component)

	retValMu.Lock()
	retVal = true
	retValMu.Unlock()

	wg.Done()
}
func handletestEventTimeout() {

}
func handletestNotificationIDNotMatching(objectData *eventbus.ObjectData) {
	name := "testTaskNotificationIdNotMatching"
	objectType := "testTypeNotificationIdNotMatching"
	resourceVersion := "testVersionNotificationIdNotMatching"
	dropTask := false

	component := &common.Component{
		Name:       "testModuleNotificationIdNotMatching",
		CompStatus: common.ComponentStatusSuccess,
	}
	TaskMan.StatusUpdated(name, objectType, resourceVersion, "NotificationIdNotMatching", dropTask, component)

	time.Sleep(100 * time.Millisecond)

	TaskMan.StatusUpdated(name, objectType, resourceVersion, objectData.NotificationID, dropTask, component)
}
func handleTestEventError(objectData *eventbus.ObjectData) {
	name := "testTaskEventError"
	objectType := "testTypeEventError"
	resourceVersion := "testVersionEventError"

	dropTask := false

	component := &common.Component{
		CompStatus: common.ComponentStatusError,
	}
	if component.Timer == 0 {
		component.Timer = 2 * time.Second
	} else {
		component.Timer *= 2
	}
	TaskMan.StatusUpdated(name, objectType, resourceVersion, objectData.NotificationID, dropTask, component)
}
func (h *moduleCiHandler) HandleEvent(eventType string, objectData *eventbus.ObjectData) {
	h.receivedEvents = append(h.receivedEvents, objectData)
	switch eventType {
	case "testEvent":
		handleTestEvent(objectData)
	case "testEventCompSuccess":
		handleTestEventCompSuccess(objectData)
	case "testEventError":
		handleTestEventError(objectData)
	case "testEventTimeout":
		handletestEventTimeout()
	case "testEventNotificationIdNotMatching":
		handletestNotificationIDNotMatching(objectData)
	default:
		log.Printf("LCI: error: Unknown event type %s", eventType)
	}
}

func TestCreateTask(t *testing.T) {
	subscriber := &eventbus.Subscriber{
		Name:     "testSubscriber",
		Ch:       make(chan interface{}),
		Quit:     make(chan bool),
		Priority: 1,
	}
	subs := []*eventbus.Subscriber{subscriber}
	tm := newTaskManager()
	tm.StartTaskManager()
	tm.CreateTask("testTask", "testType", "testVersion", subs)

	time.Sleep(100 * time.Millisecond)

	task := tm.taskQueue.Dequeue()
	assert.NotNil(t, task)
	assert.Equal(t, "testTask", task.name)
	assert.Equal(t, "testType", task.objectType)
	assert.Equal(t, "testVersion", task.resourceVersion)
	assert.Equal(t, subs, task.subs)
}

func TestCompSuccess(t *testing.T) {
	var wg sync.WaitGroup

	TaskMan.StartTaskManager()
	handler := &moduleCiHandler{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		eventbus.EBus.StartSubscriber("testModuleCompSuccess", "testEventCompSuccess", 1, handler)
	}()

	wg.Wait()

	subscribers := eventbus.EBus.GetSubscribers("testEventCompSuccess")
	if len(subscribers) == 0 {
		t.Fatalf("No subscribers found for event type 'testEvent'")
	}
	TaskMan.CreateTask("testTaskCompSuccess", "testTypeCompSuccess", "testVersionCompSuccess", subscribers)

	select {
	case task := <-TaskMan.taskQueue.channel:
		if task.name == "testTaskCompSuccess" {
			t.Errorf("assert failed:")
		}
	default:
	}
}

func TestCompError(t *testing.T) {
	var wg sync.WaitGroup

	TaskMan.StartTaskManager()

	handler := &moduleCiHandler{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		eventbus.EBus.StartSubscriber("testModuleEventError", "testEventError", 1, handler)
	}()

	wg.Wait()

	subscribers := eventbus.EBus.GetSubscribers("testEventError")

	if len(subscribers) == 0 {
		t.Fatalf("No subscribers found for event type 'testEventError'")
	}

	TaskMan.CreateTask("testTaskEventError", "testTypeEventError", "testVersionEventError", subscribers)

	task := TaskMan.taskQueue.Dequeue()

	assert.NotNil(t, task, "Task should not be nil")
	assert.Equal(t, "testTaskEventError", task.name, "Task name should match")
	assert.Equal(t, "testTypeEventError", task.objectType, "Task object type should match")
	assert.Equal(t, "testVersionEventError", task.resourceVersion, "Task resource version should match")
	assert.Equal(t, subscribers, task.subs, "Task subscribers should match")
}

func TestTimeout(t *testing.T) {
	var wg sync.WaitGroup

	handler := &moduleCiHandler{}
	TaskMan.StartTaskManager()
	wg.Add(1)
	go func() {
		defer wg.Done()
		eventbus.EBus.StartSubscriber("testModuleTimeout", "testEventTimeout", 1, handler)
	}()

	// Wait for both the TaskManager and the subscriber to be started
	wg.Wait()
	subscribers := eventbus.EBus.GetSubscribers("testEventTimeout")
	if len(subscribers) == 0 {
		t.Fatalf("No subscribers found for event type 'testEventTimeout'")
	}
	TaskMan.CreateTask("testTaskTimeout", "testTypeTimeout", "testVersionTimeout", subscribers)

	time.Sleep(35 * time.Second)

	task := TaskMan.taskQueue.Dequeue()

	assert.NotNil(t, task)
	assert.Equal(t, "testTaskTimeout", task.name)
	assert.Equal(t, "testTypeTimeout", task.objectType)
	assert.Equal(t, "testVersionTimeout", task.resourceVersion)
	assert.Equal(t, subscribers, task.subs)
}

func TestNotificationIdNotMatching(t *testing.T) {
	var wg sync.WaitGroup

	// Start the TaskManager in a separate goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		TaskMan.StartTaskManager()
	}()

	handler := &moduleCiHandler{}
	// Start the subscriber and wait for it to be ready
	wg.Add(1)
	go func() {
		defer wg.Done()
		eventbus.EBus.StartSubscriber("testModuleNotificationIdNotMatching", "testEventNotificationIdNotMatching", 1, handler)
	}()

	// Wait for both the TaskManager and the subscriber to be started
	wg.Wait()

	for i := 0; i < cap(TaskMan.taskStatusChan); i++ {
		TaskMan.taskStatusChan <- &TaskStatus{}
	}

	subscribers := eventbus.EBus.GetSubscribers("testEventNotificationIdNotMatching")
	if len(subscribers) == 0 {
		t.Fatalf("No subscribers found for event type 'testEvent'")
	}
	TaskMan.CreateTask("testTaskNotificationIdNotMatching", "testTypeNotificationIdNotMatching", "testVersionNotificationIdNotMatching", subscribers)

	time.Sleep(500 * time.Millisecond)

	select {
	case task := <-TaskMan.taskQueue.channel:
		if task.name == "testTask" {
			t.Errorf("assert failed:")
		}
	default:
	}

	for len(TaskMan.taskStatusChan) > 0 {
		<-TaskMan.taskStatusChan
	}
}

func TestStatusUpdatedChannelNotAvailable(t *testing.T) {
	var wg sync.WaitGroup

	wg.Add(1)

	go handleTestEventbusy(&wg)

	wg.Wait()
	actualRetVal := retVal

	assert.Equal(t, true, actualRetVal)
}
